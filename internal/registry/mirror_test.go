package registry

import "testing"

// makeIdx returns an initialised RegistryIndex for testing.
func makeIdx() *RegistryIndex {
	return &RegistryIndex{
		Skills:    map[string]RegistryEntry{},
		Commands:  map[string]RegistryEntry{},
		Agents:    map[string]RegistryEntry{},
		Skillsets: map[string]SkillsetEntry{},
	}
}

func TestMergeFrom_StampsSourceOnNewEntries(t *testing.T) {
	primary := makeIdx()
	primary.Skills["primary-only"] = RegistryEntry{Description: "p"}

	mirror := makeIdx()
	mirror.Skills["mirror-only"] = RegistryEntry{Description: "m"}

	primary.MergeFrom(mirror, "github:vercel-labs/agent-skills")

	if got := primary.Skills["primary-only"].Source; got != "" {
		t.Errorf("primary entry Source = %q, want empty", got)
	}
	if got := primary.Skills["mirror-only"].Source; got != "github:vercel-labs/agent-skills" {
		t.Errorf("mirror entry Source = %q, want mirror URL", got)
	}
}

func TestMergeFrom_PrimaryWinsCollisionPreservesPrimarySource(t *testing.T) {
	primary := makeIdx()
	primary.Skills["foo"] = RegistryEntry{Description: "primary version"}

	mirror := makeIdx()
	mirror.Skills["foo"] = RegistryEntry{Description: "mirror version"}

	primary.MergeFrom(mirror, "github:other/repo")

	got := primary.Skills["foo"]
	if got.Description != "primary version" {
		t.Errorf("primary should win on collision, got description %q", got.Description)
	}
	if got.Source != "" {
		t.Errorf("primary Source must remain empty after collision, got %q", got.Source)
	}
}

func TestMergeFrom_ChainedMirrorsRecordFirstContributor(t *testing.T) {
	primary := makeIdx()
	mirrorA := makeIdx()
	mirrorA.Skills["chain-skill"] = RegistryEntry{Description: "from A"}
	mirrorB := makeIdx()
	mirrorB.Skills["chain-skill"] = RegistryEntry{Description: "from B"}

	// First merge stamps with A's URL; B's collision is ignored.
	primary.MergeFrom(mirrorA, "github:source-a/skills")
	primary.MergeFrom(mirrorB, "github:source-b/skills")

	got := primary.Skills["chain-skill"]
	if got.Source != "github:source-a/skills" {
		t.Errorf("Source = %q, want first contributor github:source-a/skills", got.Source)
	}
	if got.Description != "from A" {
		t.Errorf("First contributor wins on description; got %q", got.Description)
	}
}

func TestMergeFrom_MergesCommandsAgentsAndSkillsets(t *testing.T) {
	primary := makeIdx()
	mirror := makeIdx()
	mirror.Commands["deploy"] = RegistryEntry{Description: "cmd"}
	mirror.Agents["reviewer"] = RegistryEntry{Description: "agent"}
	mirror.Skillsets["pack"] = SkillsetEntry{Description: "set"}

	primary.MergeFrom(mirror, "github:m/r")

	if got := primary.Commands["deploy"].Source; got != "github:m/r" {
		t.Errorf("commands provenance = %q", got)
	}
	if got := primary.Agents["reviewer"].Source; got != "github:m/r" {
		t.Errorf("agents provenance = %q", got)
	}
	if got := primary.Skillsets["pack"].Source; got != "github:m/r" {
		t.Errorf("skillsets provenance = %q", got)
	}
}

func TestMergeFrom_EmptyMirrorURLLeavesSourceEmpty(t *testing.T) {
	primary := makeIdx()
	mirror := makeIdx()
	mirror.Skills["foo"] = RegistryEntry{Description: "x"}

	primary.MergeFrom(mirror, "")

	if got := primary.Skills["foo"].Source; got != "" {
		t.Errorf("empty otherURL should produce empty Source, got %q", got)
	}
}
