package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/useamaru/amaru/internal/types"
)

const (
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
	maxConcurrent = 10 // max parallel file downloads per directory
)

// ClientFactory creates a Client for a given registry URL with no authentication.
// Used for resolving mirror registries.
type ClientFactory func(url string) (Client, error)

// indexSource encodes how a registry's index was obtained.
// It is the orthogonal partner of Layout (path math) — a registry can be
// (LayoutFlat, sourceIndexed) [amaru-native v2], (LayoutFlat, sourceSynthesized)
// [foreign Vercel-shaped repos], or (LayoutNested, sourceIndexed) [legacy amaru].
// (LayoutNested, sourceSynthesized) is not a real combination — synthesized
// always implies flat, since the synthesizer reads top-level <type>s/ dirs.
type indexSource int

const (
	sourceUnknown     indexSource = iota
	sourceIndexed                 // amaru_registry.json was parsed
	sourceSynthesized             // index built from a top-level skills/ walk
)

// rateLimiter tracks a global backoff state shared across all concurrent requests
// from a single client. When any request hits a 429, all goroutines pause.
type rateLimiter struct {
	mu         sync.Mutex
	pauseUntil time.Time
}

func (r *rateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	until := r.pauseUntil
	r.mu.Unlock()

	d := time.Until(until)
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (r *rateLimiter) backoff(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := time.Now().Add(d)
	if next.After(r.pauseUntil) {
		r.pauseUntil = next
	}
}

// GitHubClient implements Client using the GitHub API.
//
// After FetchIndex completes, layout and source describe the (layout, source)
// pair detected for this registry. layout drives path resolution for downloads;
// source distinguishes amaru-native registries (which carry manifests, skillsets,
// per-item version tags) from foreign synthesized registries (which don't).
type GitHubClient struct {
	Owner         string
	Repo          string
	Auth          Authenticator
	rl            *rateLimiter
	mirrorFactory ClientFactory
	layout        Layout
	source        indexSource
}

// NewGitHubClient creates a new GitHub registry client from a URL like "github:org/repo".
func NewGitHubClient(url string, auth Authenticator) (*GitHubClient, error) {
	owner, repo, err := parseGitHubURL(url)
	if err != nil {
		return nil, err
	}
	return &GitHubClient{
		Owner:  owner,
		Repo:   repo,
		Auth:   auth,
		rl:     &rateLimiter{},
		source: sourceUnknown,
	}, nil
}

// WithMirrorFactory sets the factory used to resolve mirror URLs in registry indexes.
func (c *GitHubClient) WithMirrorFactory(f ClientFactory) *GitHubClient {
	c.mirrorFactory = f
	return c
}

// parseGitHubURL parses various GitHub URL formats into owner and repo.
// Supported formats:
//   - github:org/repo (canonical shorthand)
//   - https://github.com/org/repo[.git]
//   - http://github.com/org/repo[.git] (auto-upgraded to HTTPS)
//   - git@github.com:org/repo[.git] (SSH colon syntax)
//   - ssh://git@github.com/org/repo[.git] (SSH URL syntax)
//   - github.com/org/repo[.git] (bare domain)
func parseGitHubURL(rawURL string) (string, string, error) {
	lower := strings.ToLower(rawURL)

	// Canonical shorthand: github:org/repo
	if strings.HasPrefix(lower, "github:") {
		return extractOwnerRepo(rawURL[len("github:"):], rawURL)
	}

	// SSH colon syntax: git@github.com:org/repo[.git]
	if strings.HasPrefix(lower, "git@github.com:") {
		return extractOwnerRepo(rawURL[len("git@github.com:"):], rawURL)
	}

	// SSH URL syntax: ssh://git@github.com/org/repo[.git]
	if strings.HasPrefix(lower, "ssh://git@github.com/") {
		return extractOwnerRepo(rawURL[len("ssh://git@github.com/"):], rawURL)
	}

	// HTTP: auto-upgrade to HTTPS (fall through)
	if strings.HasPrefix(lower, "http://github.com/") {
		rawURL = "https://github.com/" + rawURL[len("http://github.com/"):]
		lower = strings.ToLower(rawURL)
	}

	// Bare domain: github.com/org/repo (fall through to HTTPS)
	if strings.HasPrefix(lower, "github.com/") {
		rawURL = "https://" + rawURL
		lower = strings.ToLower(rawURL)
	}

	// HTTPS: https://github.com/org/repo[.git]
	if strings.HasPrefix(lower, "https://github.com/") {
		return extractOwnerRepo(rawURL[len("https://github.com/"):], rawURL)
	}

	// Non-GitHub SSH hosts
	if strings.HasPrefix(lower, "git@") || strings.HasPrefix(lower, "ssh://") {
		return "", "", fmt.Errorf("unsupported URL format: %s (only GitHub URLs are supported)", rawURL)
	}

	return "", "", fmt.Errorf("unsupported URL format: %s (expected github:org/repo or https://github.com/org/repo)", rawURL)
}

// extractOwnerRepo extracts owner and repo from a "org/repo[.git]" path fragment.
// Rejects URLs with extra path segments (e.g., org/repo/tree/main).
func extractOwnerRepo(path, originalURL string) (string, string, error) {
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid github URL: %s (expected org/repo)", originalURL)
	}
	if len(parts) == 3 {
		return "", "", fmt.Errorf("invalid github URL: %s (unexpected path segments after org/repo)", originalURL)
	}
	return parts[0], parts[1], nil
}

// NormalizeURL converts any accepted GitHub URL format to the canonical "github:org/repo" form.
func NormalizeURL(url string) (string, error) {
	owner, repo, err := parseGitHubURL(url)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("github:%s/%s", owner, repo), nil
}

func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "API returned 404")
}

func (c *GitHubClient) apiRequest(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/%s", c.Owner, c.Repo, path)

	token, err := c.Auth.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	var lastErr error
	for attempt := range maxRetries {
		// Check shared rate limiter before each attempt
		if err := c.rl.wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("API request failed: %w", err)
			if attempt < maxRetries-1 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryBaseWait << attempt):
				}
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		if !isRetryable(resp.StatusCode) {
			return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
		}

		lastErr = fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
		if attempt < maxRetries-1 {
			wait := retryBaseWait << attempt
			if resp.StatusCode == http.StatusTooManyRequests {
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := time.ParseDuration(ra + "s"); err == nil {
						wait = secs
					}
				}
				// Signal all concurrent goroutines to pause
				c.rl.backoff(wait)
			}
			// Wait via shared rate limiter (picks up backoff set above or by other goroutines)
			if err := c.rl.wait(ctx); err != nil {
				return nil, err
			}
		}
	}

	return nil, lastErr
}

// FetchIndex auto-detects the registry's (layout, source) pair and returns the index.
// Tries amaru_registry.json first (sourceIndexed); on 404 falls back to a top-level
// skills/ walk (sourceSynthesized, always flat). If the index declares mirrors,
// fetches and merges them (primary registry wins on conflict).
func (c *GitHubClient) FetchIndex(ctx context.Context) (*RegistryIndex, error) {
	var index *RegistryIndex

	data, err := c.getFileContent(ctx, "amaru_registry.json", "")
	if err == nil {
		var idx RegistryIndex
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, fmt.Errorf("parsing registry index: %w", err)
		}
		initIndex(&idx)
		layout, layoutErr := LayoutFor(&idx)
		if layoutErr != nil {
			return nil, fmt.Errorf("registry has unsupported layout: %w", layoutErr)
		}
		c.layout = layout
		c.source = sourceIndexed
		if layout.IsLegacy() {
			emitLegacyLayoutWarning(c.Owner, c.Repo)
		}
		index = &idx
	} else if isNotFound(err) {
		idx, synthErr := c.fetchSynthesizedIndex(ctx)
		if synthErr != nil {
			return nil, fmt.Errorf("registry has neither amaru_registry.json nor any of skills/commands/agents at the root: %w", synthErr)
		}
		// Synthesized indexes are always flat — the synthesizer reads top-level
		// <type>s/ directories, so there is no nested-and-synthesized combination.
		c.layout = LayoutFlat
		c.source = sourceSynthesized
		index = idx
	} else {
		return nil, fmt.Errorf("fetching registry index: %w", err)
	}

	// Resolve mirrors — fetch each mirror's index and merge (primary wins)
	if c.mirrorFactory != nil {
		for _, mirrorURL := range index.Mirrors {
			mirrorClient, err := c.mirrorFactory(mirrorURL)
			if err != nil {
				continue
			}
			mirrorIdx, err := mirrorClient.FetchIndex(ctx)
			if err != nil {
				continue
			}
			index.MergeFrom(mirrorIdx, mirrorURL)
		}
	}

	return index, nil
}

func initIndex(idx *RegistryIndex) {
	if idx.Skills == nil {
		idx.Skills = make(map[string]RegistryEntry)
	}
	if idx.Commands == nil {
		idx.Commands = make(map[string]RegistryEntry)
	}
	if idx.Agents == nil {
		idx.Agents = make(map[string]RegistryEntry)
	}
	if idx.Skillsets == nil {
		idx.Skillsets = make(map[string]SkillsetEntry)
	}
}

// fetchSynthesizedIndex (formerly fetchVercelIndex) builds a RegistryIndex from
// top-level skills/, commands/, agents/ in a foreign repo with no
// amaru_registry.json. Description content is read from <TYPE>.md (uppercase,
// preferred — Anthropic convention) and falls back to <type>.md (lowercase).
// Files are fetched in parallel per directory. A registry with none of the
// three top-level dirs is rejected; partial coverage is fine.
func (c *GitHubClient) fetchSynthesizedIndex(ctx context.Context) (*RegistryIndex, error) {
	idx := &RegistryIndex{
		Skills:    make(map[string]RegistryEntry),
		Commands:  make(map[string]RegistryEntry),
		Agents:    make(map[string]RegistryEntry),
		Skillsets: make(map[string]SkillsetEntry),
	}

	// scan one of the three known types; missing dirs contribute nothing.
	var foundAny bool
	for _, t := range []types.ItemType{types.Skill, types.Command, types.Agent} {
		entries, err := c.scanSynthesizedType(ctx, t)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, err
		}
		if len(entries) == 0 {
			continue
		}
		foundAny = true
		switch t {
		case types.Skill:
			idx.Skills = entries
		case types.Command:
			idx.Commands = entries
		case types.Agent:
			idx.Agents = entries
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no skill/command/agent directories found at the repository root")
	}

	return idx, nil
}

// scanSynthesizedType lists <type>s/ at the repo root and reads each child's
// content frontmatter (uppercase preferred, lowercase fallback) to populate
// the description.
func (c *GitHubClient) scanSynthesizedType(ctx context.Context, t types.ItemType) (map[string]RegistryEntry, error) {
	body, err := c.apiRequest(ctx, "contents/"+t.DirName())
	if err != nil {
		return nil, err
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s directory: %w", t.DirName(), err)
	}

	var dirs []string
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e.Name)
		}
	}
	if len(dirs) == 0 {
		return map[string]RegistryEntry{}, nil
	}

	descriptions := make([]string, len(dirs))
	errs := make([]error, len(dirs))

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i, name := range dirs {
		wg.Add(1)
		go func(idx int, itemName string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			descriptions[idx] = c.fetchSynthesizedDescription(ctx, t, itemName)
		}(i, name)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}

	out := make(map[string]RegistryEntry, len(dirs))
	for i, d := range dirs {
		out[d] = RegistryEntry{Description: descriptions[i]}
	}
	return out, nil
}

// fetchSynthesizedDescription reads the per-item content file's frontmatter.
// Tries <TYPE>.md first (Anthropic convention), then <type>.md (amaru-native).
// Missing → empty description (no error).
func (c *GitHubClient) fetchSynthesizedDescription(ctx context.Context, t types.ItemType, name string) string {
	for _, fname := range synthesizedContentCandidates(t) {
		path := t.DirName() + "/" + name + "/" + fname
		data, err := c.getFileContent(ctx, path, "")
		if err != nil {
			continue
		}
		return parseSkillFrontmatter(data).Description
	}
	return ""
}

// synthesizedContentCandidates returns the per-type filenames probed for
// frontmatter, in preferred order.
func synthesizedContentCandidates(t types.ItemType) []string {
	upper := strings.ToUpper(string(t)) + ".md"
	lower := string(t) + ".md"
	return []string{upper, lower}
}

// skillFrontmatter holds parsed YAML frontmatter fields from a SKILL.md file.
type skillFrontmatter struct {
	Name        string
	Description string
}

// parseSkillFrontmatter extracts name and description from YAML frontmatter.
// Only reads the first value for each key (no multi-line support needed).
func parseSkillFrontmatter(data []byte) skillFrontmatter {
	var fm skillFrontmatter
	lines := strings.Split(string(data), "\n")
	inFrontmatter := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}
		if !inFrontmatter {
			continue
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		// Strip surrounding quotes
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		switch k {
		case "name":
			fm.Name = v
		case "description":
			fm.Description = v
		}
	}

	return fm
}

// ListVersions returns all available versions for an item by listing git tags.
// Returns an empty list (not an error) if the registry has no tags for this item.
func (c *GitHubClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	// Tag format: skill/research/1.0.0 or command/dev/bootstrap/2.0.0
	prefix := itemType + "/" + name + "/"

	// Fetch all tags matching the prefix
	path := fmt.Sprintf("git/matching-refs/tags/%s", prefix)
	body, err := c.apiRequest(ctx, path)
	if err != nil {
		// No tags is normal for registries that don't use per-item version tags
		return nil, nil
	}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &refs); err != nil {
		return nil, nil // Treat parse failures as "no versions"
	}

	var versions []*semver.Version
	for _, ref := range refs {
		// ref.Ref is like "refs/tags/skill/research/1.0.3"
		tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
		vStr := strings.TrimPrefix(tag, prefix)
		v, err := semver.NewVersion(vStr)
		if err != nil {
			continue // Skip non-semver tags
		}
		versions = append(versions, v)
	}

	sort.Sort(semver.Collection(versions))
	return versions, nil
}

// DownloadFiles downloads all files for a specific item.
// Layout-aware: nested layout downloads from .amaru_registry/<type>s/<name>/,
// flat layout downloads from <type>s/<name>/. Synthesized-source registries
// (foreign repos with no amaru_registry.json) also use the flat <type>s/<name>/
// shape since the synthesizer reads top-level <type>s/ dirs.
// Always downloads from default branch — version is metadata only.
func (c *GitHubClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error) {
	dirPath := c.layout.RelativeItemPath(types.ItemType(itemType), name)
	return c.downloadDirectory(ctx, dirPath, "", "")
}

// FetchSkillsetManifest downloads the manifest.json for a skillset from the registry.
// Always fetches from the default branch. Synthesized-source registries do not
// support skillsets — callers get a clear error rather than a 404.
func (c *GitHubClient) FetchSkillsetManifest(ctx context.Context, name, _ string) (*SkillsetManifest, error) {
	if c.source == sourceSynthesized {
		return nil, fmt.Errorf("registry %s/%s has no amaru_registry.json and does not support skillsets", c.Owner, c.Repo)
	}
	filePath := c.layout.RelativeSkillsetManifestPath(name)
	data, err := c.getFileContent(ctx, filePath, "")
	if err != nil {
		return nil, fmt.Errorf("fetching skillset manifest for %q: %w", name, err)
	}

	var m SkillsetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing skillset manifest for %q: %w", name, err)
	}

	return &m, nil
}

// downloadDirectory recursively downloads all files in a directory at a given ref.
// Files within each directory level are fetched in parallel.
func (c *GitHubClient) downloadDirectory(ctx context.Context, dirPath, ref, relativeBase string) ([]File, error) {
	path := fmt.Sprintf("contents/%s", dirPath)
	if ref != "" {
		path += "?ref=" + ref
	}

	body, err := c.apiRequest(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing directory %s: %w", dirPath, err)
	}

	var entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing directory listing: %w", err)
	}

	type fileWork struct {
		apiPath string
		relPath string
	}
	type dirWork struct {
		apiPath string
		relPath string
	}

	var fileWorks []fileWork
	var dirWorks []dirWork

	for _, entry := range entries {
		relPath := entry.Name
		if relativeBase != "" {
			relPath = relativeBase + "/" + entry.Name
		}
		switch entry.Type {
		case "file":
			fileWorks = append(fileWorks, fileWork{entry.Path, relPath})
		case "dir":
			dirWorks = append(dirWorks, dirWork{entry.Path, relPath})
		}
	}

	// Fetch all files at this level in parallel
	type fileResult struct {
		file File
		err  error
	}
	fileResults := make([]fileResult, len(fileWorks))

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, fw := range fileWorks {
		wg.Add(1)
		go func(idx int, apiPath, relPath string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				fileResults[idx].err = ctx.Err()
				return
			}
			defer func() { <-sem }()

			content, err := c.getFileContent(ctx, apiPath, ref)
			if err != nil {
				fileResults[idx].err = err
				return
			}
			fileResults[idx].file = File{Path: relPath, Content: content}
		}(i, fw.apiPath, fw.relPath)
	}
	wg.Wait()

	var files []File
	for _, r := range fileResults {
		if r.err != nil {
			return nil, r.err
		}
		files = append(files, r.file)
	}

	// Recurse into subdirectories (sequential — subdirs are rare)
	for _, dw := range dirWorks {
		subFiles, err := c.downloadDirectory(ctx, dw.apiPath, ref, dw.relPath)
		if err != nil {
			return nil, err
		}
		files = append(files, subFiles...)
	}

	return files, nil
}

// getFileContent fetches a file's content from the GitHub API.
func (c *GitHubClient) getFileContent(ctx context.Context, filePath, ref string) ([]byte, error) {
	path := fmt.Sprintf("contents/%s", filePath)
	if ref != "" {
		path += "?ref=" + ref
	}

	body, err := c.apiRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	var fileResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("parsing file response: %w", err)
	}

	if fileResp.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding: %s", fileResp.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileResp.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decoding base64 content: %w", err)
	}
	return decoded, nil
}
