package workflow

import (
	"encoding/json"
	"testing"
)

func TestDefaultWorkflowSeeds(t *testing.T) {
	seeds := defaultWorkflowSeeds()
	if len(seeds) == 0 {
		t.Fatal("expected workflow seeds")
	}

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
	}
}
