package registry

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmitLegacyLayoutWarning_FiresOncePerRegistry(t *testing.T) {
	var buf bytes.Buffer
	prev := SetDeprecationWriter(&buf)
	defer SetDeprecationWriter(prev)
	ResetDeprecationState()
	defer ResetDeprecationState()

	emitLegacyLayoutWarning("acme-org", "skills")
	emitLegacyLayoutWarning("acme-org", "skills") // duplicate — should be suppressed
	emitLegacyLayoutWarning("acme-org", "skills") // and again

	count := strings.Count(buf.String(), "uses the legacy")
	if count != 1 {
		t.Errorf("expected exactly 1 warning for repeated calls to same registry, got %d.\nOutput:\n%s", count, buf.String())
	}
}

func TestEmitLegacyLayoutWarning_FiresPerDistinctRegistry(t *testing.T) {
	var buf bytes.Buffer
	prev := SetDeprecationWriter(&buf)
	defer SetDeprecationWriter(prev)
	ResetDeprecationState()
	defer ResetDeprecationState()

	emitLegacyLayoutWarning("acme-org", "skills")
	emitLegacyLayoutWarning("other-org", "things")

	out := buf.String()
	if !strings.Contains(out, "acme-org/skills") {
		t.Errorf("expected warning for acme-org/skills, got:\n%s", out)
	}
	if !strings.Contains(out, "other-org/things") {
		t.Errorf("expected warning for other-org/things, got:\n%s", out)
	}
}

func TestEmitLegacyLayoutWarning_QuietFlagSuppresses(t *testing.T) {
	var buf bytes.Buffer
	prev := SetDeprecationWriter(&buf)
	defer SetDeprecationWriter(prev)
	ResetDeprecationState()
	defer ResetDeprecationState()

	QuietDeprecations = true
	defer func() { QuietDeprecations = false }()

	emitLegacyLayoutWarning("acme-org", "skills")

	if buf.Len() != 0 {
		t.Errorf("expected no output when QuietDeprecations=true, got: %q", buf.String())
	}
}

func TestEmitLegacyLayoutWarning_NilWriterSilences(t *testing.T) {
	prev := SetDeprecationWriter(nil)
	defer SetDeprecationWriter(prev)
	ResetDeprecationState()
	defer ResetDeprecationState()

	// Should not panic with a nil writer.
	emitLegacyLayoutWarning("acme-org", "skills")
}

func TestFetchSkillsetManifest_RejectsSynthesizedSource(t *testing.T) {
	c := &GitHubClient{
		Owner:  "vercel-labs",
		Repo:   "agent-skills",
		layout: LayoutFlat,
		source: sourceSynthesized,
	}
	_, err := c.FetchSkillsetManifest(nil, "starter-pack", "")
	if err == nil {
		t.Fatal("expected error for synthesized-source FetchSkillsetManifest, got nil")
	}
	if !strings.Contains(err.Error(), "does not support skillsets") {
		t.Errorf("expected 'does not support skillsets' error, got: %v", err)
	}
}
