package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// tarballOf builds a GitHub-shaped repository tarball: every path is nested
// under one "<owner>-<repo>-<sha>/" directory that the parser must strip.
func tarballOf(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{Name: root + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		header := &tar.Header{
			Name:     root + "/" + path,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// snapshotServer serves the tarball endpoint plus the contents API, counting
// hits on each so a test can prove which path a read took.
type snapshotServer struct {
	tarballHits  atomic.Int32
	contentsHits atomic.Int32
}

func newSnapshotClient(t *testing.T, files map[string]string, serveTarball bool) (*GitHubClient, *snapshotServer) {
	t.Helper()
	counts := &snapshotServer{}
	owner, repo := "acme", "skills"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/repos/" + owner + "/" + repo + "/"
		switch {
		case r.URL.Path == base+"tarball":
			counts.tarballHits.Add(1)
			if !serveTarball {
				http.Error(w, "no tarball", http.StatusNotFound)
				return
			}
			_, _ = w.Write(tarballOf(t, owner+"-"+repo+"-abc1234", files))

		case strings.HasPrefix(r.URL.Path, base+"contents/"):
			counts.contentsHits.Add(1)
			key := strings.TrimPrefix(r.URL.Path, base+"contents/")
			if content, ok := files[key]; ok {
				_ = json.NewEncoder(w).Encode(map[string]string{
					"content":  base64.StdEncoding.EncodeToString([]byte(content)),
					"encoding": "base64",
				})
				return
			}
			var entries []map[string]string
			for path := range files {
				rel, found := strings.CutPrefix(path, key+"/")
				if !found || strings.Contains(rel, "/") {
					continue
				}
				entries = append(entries, map[string]string{"name": rel, "path": path, "type": "file"})
			}
			if len(entries) == 0 {
				http.Error(w, "API returned 404: "+key, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(entries)

		default:
			http.Error(w, "API returned 404: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return &GitHubClient{
		Owner:   owner,
		Repo:    repo,
		Auth:    &noAuthenticator{},
		rl:      &rateLimiter{},
		apiBase: srv.URL,
		layout:  LayoutFlat,
	}, counts
}

func TestParseSnapshotStripsRootDirectory(t *testing.T) {
	raw := tarballOf(t, "acme-skills-abc1234", map[string]string{
		"skills/research/SKILL.md":           "body",
		"skills/research/refs/deep/notes.md": "nested",
		"amaru_registry.json":                "{}",
	})

	snap, err := parseSnapshot(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := snap.file("skills/research/SKILL.md"); !ok {
		t.Error("expected repo-relative path without the tarball root")
	}
	if _, ok := snap.file("acme-skills-abc1234/skills/research/SKILL.md"); ok {
		t.Error("tarball root directory should have been stripped")
	}
	if !snap.hasDir("skills/research") {
		t.Error("hasDir should see a populated directory")
	}
	if snap.hasDir("skills/missing") {
		t.Error("hasDir should not invent directories")
	}
	if got := len(snap.filesUnder("skills/research")); got != 2 {
		t.Errorf("filesUnder returned %d files, want 2 (including the nested one)", got)
	}
}

func TestParseSnapshotRejectsEmptyArchive(t *testing.T) {
	if _, err := parseSnapshot(tarballOf(t, "acme-skills-abc", nil)); err == nil {
		t.Error("an archive with no files must be rejected, not served as an empty tree")
	}
}

// The whole point: a skillset with many members must cost one tarball, not one
// request per file.
func TestDownloadFilesUsesOneTarballForEveryItem(t *testing.T) {
	files := map[string]string{"amaru_registry.json": `{"amaru_version":"2","skills":{}}`}
	for i := range 5 {
		files[fmt.Sprintf("skills/skill-%d/SKILL.md", i)] = fmt.Sprintf("body %d", i)
		files[fmt.Sprintf("skills/skill-%d/reference.md", i)] = "ref"
	}
	c, counts := newSnapshotClient(t, files, true)

	for i := range 5 {
		got, err := c.DownloadFiles(context.Background(), "skill", fmt.Sprintf("skill-%d", i), "")
		if err != nil {
			t.Fatalf("skill-%d: %v", i, err)
		}
		if len(got) != 2 {
			t.Fatalf("skill-%d: got %d files, want 2", i, len(got))
		}
		if got[0].Path != "SKILL.md" || string(got[0].Content) != fmt.Sprintf("body %d", i) {
			t.Errorf("skill-%d: unexpected first file %q = %q", i, got[0].Path, got[0].Content)
		}
	}

	if n := counts.tarballHits.Load(); n != 1 {
		t.Errorf("tarball fetched %d times, want exactly 1", n)
	}
	// One contents hit is the registry index, read before any item download.
	// The 10 member files cost zero — they come out of the tarball.
	if n := counts.contentsHits.Load(); n != 1 {
		t.Errorf("contents API hit %d times, want 1 (the index only)", n)
	}
}

func TestDownloadFilesFallsBackWhenTarballUnavailable(t *testing.T) {
	files := map[string]string{
		"amaru_registry.json":      `{"amaru_version":"2","skills":{}}`,
		"skills/research/SKILL.md": "body",
	}
	c, counts := newSnapshotClient(t, files, false)

	got, err := c.DownloadFiles(context.Background(), "skill", "research", "")
	if err != nil {
		t.Fatalf("a failing tarball must fall back to the contents API, got: %v", err)
	}
	if len(got) != 1 || string(got[0].Content) != "body" {
		t.Fatalf("unexpected files: %+v", got)
	}
	if counts.contentsHits.Load() == 0 {
		t.Error("expected the contents API to serve the fallback")
	}
}

func TestSnapshotCanBeDisabled(t *testing.T) {
	t.Setenv("AMARU_NO_SNAPSHOT", "1")
	files := map[string]string{
		"amaru_registry.json":      `{"amaru_version":"2","skills":{}}`,
		"skills/research/SKILL.md": "body",
	}
	c, counts := newSnapshotClient(t, files, true)

	if _, err := c.DownloadFiles(context.Background(), "skill", "research", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := counts.tarballHits.Load(); n != 0 {
		t.Errorf("AMARU_NO_SNAPSHOT must skip the tarball, got %d hits", n)
	}
	if counts.contentsHits.Load() == 0 {
		t.Error("expected the contents API to serve the read")
	}
}

// A single-file read must not pay for a whole-repo download: FetchIndex reads
// amaru_registry.json before any item is downloaded.
func TestSingleFileReadDoesNotFetchTarball(t *testing.T) {
	files := map[string]string{
		"amaru_registry.json":      `{"amaru_version":"2","skills":{}}`,
		"skills/research/SKILL.md": "body",
	}
	c, counts := newSnapshotClient(t, files, true)

	if _, err := c.getFileContent(context.Background(), "amaru_registry.json", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := counts.tarballHits.Load(); n != 0 {
		t.Errorf("a single-file read pulled the tarball %d times, want 0", n)
	}
}

// Once the snapshot is in memory, single-file reads come from it too.
func TestSingleFileReadUsesLoadedSnapshot(t *testing.T) {
	files := map[string]string{
		"amaru_registry.json":      `{"amaru_version":"2","skills":{}}`,
		"skills/research/SKILL.md": "body",
	}
	c, counts := newSnapshotClient(t, files, true)

	if _, err := c.DownloadFiles(context.Background(), "skill", "research", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := counts.contentsHits.Load()

	got, err := c.getFileContent(context.Background(), "amaru_registry.json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != files["amaru_registry.json"] {
		t.Errorf("got %q", got)
	}
	if counts.contentsHits.Load() != before {
		t.Error("the loaded snapshot should have served the file, not the API")
	}
}
