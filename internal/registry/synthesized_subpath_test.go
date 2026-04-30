package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

// stubGithub spins up an httptest.Server that mimics the subset of the
// GitHub Contents API that fetchSynthesizedIndex consumes.
//
// dirs maps a slash path (e.g. "skills" or "skills/cloud/bigquery") to a list
// of (name, type) entries. files maps a slash path to its raw bytes. Anything
// else returns 404.
type stubGithub struct {
	dirs  map[string][]dirEntry
	files map[string][]byte
}

func newStubGithub(t *testing.T, owner, repo string, s *stubGithub) (*GitHubClient, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// path is /repos/<owner>/<repo>/contents/<path>?ref=...
		prefix := "/repos/" + owner + "/" + repo + "/contents/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, prefix)
		if dir, ok := s.dirs[key]; ok {
			_ = json.NewEncoder(w).Encode(dir)
			return
		}
		if data, ok := s.files[key]; ok {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content":  base64.StdEncoding.EncodeToString(data),
				"encoding": "base64",
			})
			return
		}
		http.Error(w, "API returned 404 not found: "+key, http.StatusNotFound)
	}))
	auth := &noAuthenticator{}
	c := &GitHubClient{
		Owner:   owner,
		Repo:    repo,
		Auth:    auth,
		rl:      &rateLimiter{},
		apiBase: srv.URL,
	}
	return c, srv.Close
}

func TestSynthesizedScan_FlatLayout_VercelStyle(t *testing.T) {
	c, cleanup := newStubGithub(t, "vercel-labs", "agent-skills", &stubGithub{
		dirs: map[string][]dirEntry{
			"skills": {
				{Name: "research", Type: "dir"},
				{Name: "deploy", Type: "dir"},
				{Name: "README.md", Type: "file"},
			},
		},
		files: map[string][]byte{
			"skills/research/SKILL.md": []byte("---\ndescription: Compress codebase context\n---\n"),
			"skills/deploy/skill.md":   []byte("---\ndescription: Deployment helper\n---\n"),
		},
	})
	defer cleanup()

	entries, err := c.scanSynthesizedType(context.Background(), types.Skill)
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if e, ok := entries["research"]; !ok || e.Description != "Compress codebase context" {
		t.Errorf("research entry: %+v", e)
	}
	if e, ok := entries["deploy"]; !ok || e.Description != "Deployment helper" {
		t.Errorf("deploy entry: %+v", e)
	}
}

func TestSynthesizedScan_NestedLayout_GoogleStyle(t *testing.T) {
	c, cleanup := newStubGithub(t, "google", "skills", &stubGithub{
		dirs: map[string][]dirEntry{
			"skills": {
				{Name: "cloud", Type: "dir"},
				{Name: "data", Type: "dir"},
			},
			"skills/cloud": {
				{Name: "bigquery", Type: "dir"},
				{Name: "spanner", Type: "dir"},
				{Name: "README.md", Type: "file"},
			},
			"skills/data": {
				{Name: "schema-design", Type: "dir"},
			},
		},
		files: map[string][]byte{
			"skills/cloud/bigquery/SKILL.md":     []byte("---\ndescription: Query BigQuery\n---\n"),
			"skills/cloud/spanner/SKILL.md":      []byte("---\ndescription: Spanner schema\n---\n"),
			"skills/data/schema-design/SKILL.md": []byte("---\ndescription: Schema design\n---\n"),
		},
	})
	defer cleanup()

	entries, err := c.scanSynthesizedType(context.Background(), types.Skill)
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}
	for _, name := range []string{"cloud/bigquery", "cloud/spanner", "data/schema-design"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("missing entry %q (have: %v)", name, entries)
		}
	}
	if d := entries["cloud/bigquery"].Description; d != "Query BigQuery" {
		t.Errorf("cloud/bigquery description = %q", d)
	}
}

func TestSynthesizedScan_MixedFlatAndNested(t *testing.T) {
	c, cleanup := newStubGithub(t, "acme", "skills", &stubGithub{
		dirs: map[string][]dirEntry{
			"skills": {
				{Name: "lint", Type: "dir"},     // flat
				{Name: "cloud", Type: "dir"},    // namespace
			},
			"skills/cloud": {
				{Name: "deploy", Type: "dir"},
			},
		},
		files: map[string][]byte{
			"skills/lint/SKILL.md":         []byte("---\ndescription: Lint\n---\n"),
			"skills/cloud/deploy/SKILL.md": []byte("---\ndescription: Cloud deploy\n---\n"),
		},
	})
	defer cleanup()

	entries, err := c.scanSynthesizedType(context.Background(), types.Skill)
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	if _, ok := entries["lint"]; !ok {
		t.Errorf("flat skill 'lint' missing")
	}
	if _, ok := entries["cloud/deploy"]; !ok {
		t.Errorf("nested skill 'cloud/deploy' missing")
	}
}

func TestSynthesizedScan_DepthCapAtOne(t *testing.T) {
	// skills/foo/bar/baz/SKILL.md — three levels deep, must be ignored.
	c, cleanup := newStubGithub(t, "deep", "skills", &stubGithub{
		dirs: map[string][]dirEntry{
			"skills": {
				{Name: "foo", Type: "dir"},
			},
			"skills/foo": {
				{Name: "bar", Type: "dir"},
			},
			"skills/foo/bar": {
				{Name: "baz", Type: "dir"},
			},
		},
		files: map[string][]byte{
			"skills/foo/bar/baz/SKILL.md": []byte("---\ndescription: too deep\n---\n"),
		},
	})
	defer cleanup()

	entries, err := c.scanSynthesizedType(context.Background(), types.Skill)
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected zero entries (depth >1 not supported), got %+v", entries)
	}
}

func TestSynthesizedScan_NamespaceWithMixedGrandchildren(t *testing.T) {
	// One grandchild has a content file, one doesn't. Only the hit is returned.
	c, cleanup := newStubGithub(t, "x", "y", &stubGithub{
		dirs: map[string][]dirEntry{
			"skills": {
				{Name: "ns", Type: "dir"},
			},
			"skills/ns": {
				{Name: "real", Type: "dir"},
				{Name: "empty", Type: "dir"}, // no content file
			},
			"skills/ns/empty": {
				{Name: "README.md", Type: "file"},
			},
		},
		files: map[string][]byte{
			"skills/ns/real/SKILL.md": []byte("---\ndescription: real one\n---\n"),
		},
	})
	defer cleanup()

	entries, err := c.scanSynthesizedType(context.Background(), types.Skill)
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if _, ok := entries["ns/real"]; !ok {
		t.Errorf("expected ns/real, got %+v", entries)
	}
}
