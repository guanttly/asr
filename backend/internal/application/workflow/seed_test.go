package workflow

import (
	"encoding/json"
	"testing"

	domain "github.com/lgt/asr/internal/domain/workflow"
)

func TestDefaultWorkflowSeeds(t *testing.T) {
	seeds := defaultWorkflowSeeds()
	if len(seeds) == 0 {
		t.Fatal("expected workflow seeds")
	}

	voiceSeedFound := false

	seenNames := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		if seed.Name == "" {
			t.Fatal("seed name should not be empty")
		}
		if _, exists := seenNames[seed.Name]; exists {
			t.Fatalf("duplicate seed name: %s", seed.Name)
		}
		seenNames[seed.Name] = struct{}{}

		if len(seed.Nodes) == 0 {
			t.Fatalf("seed %s should include nodes", seed.Name)
		}

		for _, node := range seed.Nodes {
			if !node.NodeType.Valid() {
				t.Fatalf("seed %s contains invalid node type: %s", seed.Name, node.NodeType)
			}
			if !json.Valid([]byte(node.Config)) {
				t.Fatalf("seed %s contains invalid node config for node type %s", seed.Name, node.NodeType)
			}
		}

		if seed.Name == "语音控制工作流" {
			voiceSeedFound = true
			if len(seed.Nodes) != 2 {
				t.Fatalf("voice seed should include exactly 2 fixed nodes, got %d", len(seed.Nodes))
			}
			if seed.Nodes[0].NodeType != domain.NodeVoiceWake {
				t.Fatalf("voice seed first node should be voice_wake, got %s", seed.Nodes[0].NodeType)
			}
			if seed.Nodes[1].NodeType != domain.NodeVoiceIntent {
				t.Fatalf("voice seed second node should be voice_intent, got %s", seed.Nodes[1].NodeType)
			}
		}
	}

	if !voiceSeedFound {
		t.Fatal("expected built-in voice control workflow seed")
	}
}
