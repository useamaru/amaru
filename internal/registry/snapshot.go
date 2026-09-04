package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// maxSnapshotBytes caps how much a single registry tarball may expand to.
// Registries are markdown trees measured in single-digit megabytes; the cap
// exists so a hostile or misconfigured registry can't exhaust memory.
const maxSnapshotBytes = 256 << 20

// snapshot is one registry repository's default branch, fetched as a single
// tarball and kept in memory for the process' lifetime.
//
// The GitHub contents API costs one request per file, and round-trip latency
// dominates everything else: a skillset with dozens of members spent minutes
// waiting on ~130 sequential-ish requests. One tarball replaces all of them.
type snapshot struct {
	files map[string][]byte // repo-relative path → content
}

// file returns the content stored at a repo-relative path.
//
// path is the repo-relative path (no leading slash).
// Returns the content and whether the snapshot holds that path.
func (s *snapshot) file(path string) ([]byte, bool) {
	content, ok := s.files[path]
	return content, ok
}

// hasDir reports whether the snapshot holds anything under a directory.
//
// dir is the repo-relative directory path, without a trailing slash.
func (s *snapshot) hasDir(dir string) bool {
	prefix := dir + "/"
	for path := range s.files {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// filesUnder returns every file below a directory, as paths relative to it.
//
// dir is the repo-relative directory path, without a trailing slash.
// Returns a map of relative path → content; empty when the directory is absent.
func (s *snapshot) filesUnder(dir string) map[string][]byte {
	prefix := dir + "/"
	out := make(map[string][]byte)
	for path, content := range s.files {
		if rel, found := strings.CutPrefix(path, prefix); found {
			out[rel] = content
		}
	}
	return out
}

// snapshotDisabled reports whether the tarball path is turned off, which forces
// every read back onto the per-file contents API.
func snapshotDisabled() bool {
	return os.Getenv("AMARU_NO_SNAPSHOT") != ""
}

// loadSnapshot fetches the registry's default-branch tarball once per client.
//
// ctx is the cancellation context.
// Returns the snapshot, or nil when snapshots are disabled or the fetch failed
// — callers treat a nil snapshot as "use the API", never as an error.
func (c *GitHubClient) loadSnapshot(ctx context.Context) *snapshot {
	if snapshotDisabled() {
		return nil
	}
	c.snapOnce.Do(func() {
		body, err := c.apiRequest(ctx, "tarball")
		if err != nil {
			return
		}
		snap, err := parseSnapshot(body)
		if err != nil {
			return
		}
		c.snap = snap
	})
	return c.snap
}

// cachedSnapshot returns the snapshot only if a previous call already fetched
// it. Single-file reads use this: paying a whole-repo download to serve one
// file would be slower than the request it replaces.
func (c *GitHubClient) cachedSnapshot() *snapshot {
	if snapshotDisabled() {
		return nil
	}
	return c.snap
}

// parseSnapshot expands a GitHub repository tarball into an in-memory tree.
//
// body is the gzipped tar as returned by the repos/{owner}/{repo}/tarball
// endpoint. GitHub wraps everything in a single "<owner>-<repo>-<sha>/"
// directory, which is stripped so paths are repo-relative.
// Returns the snapshot, or an error when the archive is unreadable or exceeds
// maxSnapshotBytes.
func parseSnapshot(body []byte) (*snapshot, error) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reading tarball: %w", err)
	}
	defer gz.Close()

	files := make(map[string][]byte)
	var total int64
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tarball entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		_, path, found := strings.Cut(header.Name, "/")
		if !found || path == "" {
			continue
		}

		total += header.Size
		if total > maxSnapshotBytes {
			return nil, fmt.Errorf("registry tarball exceeds %d bytes", int64(maxSnapshotBytes))
		}

		content, err := io.ReadAll(io.LimitReader(tr, maxSnapshotBytes))
		if err != nil {
			return nil, fmt.Errorf("reading %s from tarball: %w", path, err)
		}
		files[path] = content
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("registry tarball is empty")
	}
	return &snapshot{files: files}, nil
}
