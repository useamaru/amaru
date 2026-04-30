package registry

import (
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestSynthesizedContentCandidates(t *testing.T) {
	tests := []struct {
		t    types.ItemType
		want []string
	}{
		{types.Skill, []string{"SKILL.md", "skill.md"}},
		{types.Command, []string{"COMMAND.md", "command.md"}},
		{types.Agent, []string{"AGENT.md", "agent.md"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.t), func(t *testing.T) {
			got := synthesizedContentCandidates(tt.t)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("candidate[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
