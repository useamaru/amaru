package registry

import (
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		// Canonical shorthand
		{url: "github:acme-org/acme-skills", wantOwner: "acme-org", wantRepo: "acme-skills"},
		// HTTPS
		{url: "https://github.com/acme-org/platform-skills", wantOwner: "acme-org", wantRepo: "platform-skills"},
		{url: "https://github.com/acme-org/platform-skills.git", wantOwner: "acme-org", wantRepo: "platform-skills"},
		// SSH colon syntax
		{url: "git@github.com:Visio-ai/ai_registry.git", wantOwner: "Visio-ai", wantRepo: "ai_registry"},
		{url: "git@github.com:org/repo", wantOwner: "org", wantRepo: "repo"},
		// SSH URL syntax
		{url: "ssh://git@github.com/org/repo.git", wantOwner: "org", wantRepo: "repo"},
		{url: "ssh://git@github.com/org/repo", wantOwner: "org", wantRepo: "repo"},
		// HTTP auto-upgrade
		{url: "http://github.com/org/repo", wantOwner: "org", wantRepo: "repo"},
		{url: "http://github.com/org/repo.git", wantOwner: "org", wantRepo: "repo"},
		// Bare domain
		{url: "github.com/org/repo", wantOwner: "org", wantRepo: "repo"},
		{url: "github.com/org/repo.git", wantOwner: "org", wantRepo: "repo"},
		// Case insensitive prefixes
		{url: "Git@GitHub.com:org/repo.git", wantOwner: "org", wantRepo: "repo"},
		{url: "HTTPS://GITHUB.COM/org/repo", wantOwner: "org", wantRepo: "repo"},
		// Trailing slashes
		{url: "github:org/repo/", wantOwner: "org", wantRepo: "repo"},
		{url: "git@github.com:org/repo/", wantOwner: "org", wantRepo: "repo"},
		// Errors: invalid formats
		{url: "github:invalid", wantErr: true},
		{url: "github:/repo", wantErr: true},
		// Errors: non-GitHub hosts
		{url: "gitlab:org/repo", wantErr: true},
		{url: "git@gitlab.com:org/repo.git", wantErr: true},
		{url: "ssh://git@bitbucket.org/org/repo", wantErr: true},
		// Errors: extra path segments
		{url: "https://github.com/org/repo/tree/main", wantErr: true},
		{url: "git@github.com:org/repo/extra.git", wantErr: true},
		// Errors: empty parts
		{url: "git@github.com:/repo.git", wantErr: true},
		{url: "git@github.com:org/.git", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitHubURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if owner != tt.wantOwner {
					t.Errorf("owner = %s, want %s", owner, tt.wantOwner)
				}
				if repo != tt.wantRepo {
					t.Errorf("repo = %s, want %s", repo, tt.wantRepo)
				}
			}
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "github:org/repo", want: "github:org/repo"},
		{input: "git@github.com:org/repo.git", want: "github:org/repo"},
		{input: "ssh://git@github.com/org/repo.git", want: "github:org/repo"},
		{input: "https://github.com/org/repo.git", want: "github:org/repo"},
		{input: "http://github.com/org/repo", want: "github:org/repo"},
		{input: "github.com/org/repo", want: "github:org/repo"},
		{input: "git@gitlab.com:org/repo.git", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		method     string
		wantMethod string
		wantErr    bool
	}{
		{"github", "gh CLI", false},
		{"token", "env token", false},
		{"none", "none", false},
		{"oauth", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			auth, err := NewAuthenticator(tt.method, "main")
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAuthenticator(%q) error = %v, wantErr %v", tt.method, err, tt.wantErr)
				return
			}
			if !tt.wantErr && auth.Method() != tt.wantMethod {
				t.Errorf("Method() = %s, want %s", auth.Method(), tt.wantMethod)
			}
		})
	}
}

func TestSkillsetManifestToSkillsetItems(t *testing.T) {
	tests := []struct {
		name     string
		manifest SkillsetManifest
		want     int
	}{
		{
			name: "skills only",
			manifest: SkillsetManifest{
				Skills: []string{"research", "plan"},
			},
			want: 2,
		},
		{
			name: "mixed types",
			manifest: SkillsetManifest{
				Skills:   []string{"research"},
				Commands: []string{"bootstrap"},
				Agents:   []string{"coder"},
			},
			want: 3,
		},
		{
			name: "empty manifest",
			manifest: SkillsetManifest{},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := tt.manifest.ToSkillsetItems()
			if len(items) != tt.want {
				t.Errorf("got %d items, want %d", len(items), tt.want)
			}

			// Verify types are correct
			for _, item := range items {
				switch item.Type {
				case "skill", "command", "agent":
					// ok
				default:
					t.Errorf("unexpected item type: %s", item.Type)
				}
			}
		})
	}
}

func TestSkillsetManifestPreservesOrder(t *testing.T) {
	m := SkillsetManifest{
		Skills:   []string{"a-skill", "b-skill"},
		Commands: []string{"a-cmd"},
		Agents:   []string{"a-agent"},
	}
	items := m.ToSkillsetItems()

	// Skills come first, then commands, then agents
	expected := []struct {
		typ  string
		name string
	}{
		{"skill", "a-skill"},
		{"skill", "b-skill"},
		{"command", "a-cmd"},
		{"agent", "a-agent"},
	}

	if len(items) != len(expected) {
		t.Fatalf("got %d items, want %d", len(items), len(expected))
	}
	for i, e := range expected {
		if items[i].Type != e.typ || items[i].Name != e.name {
			t.Errorf("item[%d] = %s/%s, want %s/%s", i, items[i].Type, items[i].Name, e.typ, e.name)
		}
	}
}

func TestRegistryIndexEntriesForType(t *testing.T) {
	idx := &RegistryIndex{
		Skills:   map[string]RegistryEntry{"research": {Latest: "1.0.0"}},
		Commands: map[string]RegistryEntry{"bootstrap": {Latest: "2.0.0"}},
		Agents:   map[string]RegistryEntry{"coder": {Latest: "1.0.0"}},
	}

	tests := []struct {
		name     string
		itemType types.ItemType
		wantKey  string
	}{
		{"skill", types.Skill, "research"},
		{"command", types.Command, "bootstrap"},
		{"agent", types.Agent, "coder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := idx.EntriesForType(tt.itemType)
			if entries == nil {
				t.Fatal("expected non-nil entries")
			}
			if _, ok := entries[tt.wantKey]; !ok {
				t.Errorf("expected key %s", tt.wantKey)
			}
		})
	}

	if idx.EntriesForType(types.ItemType("widget")) != nil {
		t.Error("expected nil for unknown type")
	}
}
