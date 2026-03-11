package runner

import (
	"reflect"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveRunSkillFilter(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "browser-agent", Skills: []string{"browser-ops"}},
			},
		},
	}

	got := resolveRunSkillFilter(RunEmbeddedPiAgentParams{
		AgentID: "browser-agent",
		Config:  cfg,
	})
	if !reflect.DeepEqual(got, []string{"browser-ops"}) {
		t.Fatalf("agent-config SkillFilter = %v, want [browser-ops]", got)
	}

	got = resolveRunSkillFilter(RunEmbeddedPiAgentParams{
		AgentID:     "browser-agent",
		Config:      cfg,
		SkillFilter: []string{"explicit-skill"},
	})
	if !reflect.DeepEqual(got, []string{"explicit-skill"}) {
		t.Fatalf("explicit SkillFilter = %v, want [explicit-skill]", got)
	}
}
