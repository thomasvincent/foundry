package plan

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/foundry-ci/foundry/internal/config"
)

// TestTopologicalSort_Simple verifies basic topological ordering.
func TestTopologicalSort_Simple(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{ID: "lint", Type: "shell", Deps: []string{}},
		{ID: "test", Type: "shell", Deps: []string{"lint"}},
	}

	order, err := TopologicalSort(steps)
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 2 {
		t.Errorf("expected 2 steps, got %d", len(order))
	}

	if order[0] != "lint" || order[1] != "test" {
		t.Errorf("expected order [lint, test], got %v", order)
	}
}

// TestTopologicalSort_NoDeps verifies deterministic alphabetical ordering when no dependencies exist.
func TestTopologicalSort_NoDeps(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{ID: "c", Type: "shell", Deps: []string{}},
		{ID: "a", Type: "shell", Deps: []string{}},
		{ID: "b", Type: "shell", Deps: []string{}},
	}

	order, err := TopologicalSort(steps)
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Errorf("expected 3 steps, got %d", len(order))
	}

	// Should be sorted alphabetically when no deps exist.
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("expected alphabetical order [a, b, c], got %v", order)
	}
}

// TestTopologicalSort_Diamond verifies deterministic ordering with a diamond dependency pattern.
// a -> b, a -> c, b -> d, c -> d
func TestTopologicalSort_Diamond(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{ID: "a", Type: "shell", Deps: []string{}},
		{ID: "b", Type: "shell", Deps: []string{"a"}},
		{ID: "c", Type: "shell", Deps: []string{"a"}},
		{ID: "d", Type: "shell", Deps: []string{"b", "c"}},
	}

	order, err := TopologicalSort(steps)
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 4 {
		t.Errorf("expected 4 steps, got %d", len(order))
	}

	// First step must be 'a', last must be 'd'.
	if order[0] != "a" {
		t.Errorf("expected first step 'a', got %q", order[0])
	}

	if order[3] != "d" {
		t.Errorf("expected last step 'd', got %q", order[3])
	}

	// Middle steps should be 'b' and 'c' in alphabetical order (deterministic tie-breaking).
	if order[1] != "b" || order[2] != "c" {
		t.Errorf("expected middle steps [b, c], got [%q, %q]", order[1], order[2])
	}
}

// TestTopologicalSort_Cycle verifies that cyclic dependencies are detected.
func TestTopologicalSort_Cycle(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{ID: "a", Type: "shell", Deps: []string{"b"}},
		{ID: "b", Type: "shell", Deps: []string{"a"}},
	}

	_, err := TopologicalSort(steps)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}

	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

// TestTopologicalSort_Empty verifies that empty step lists return empty order.
func TestTopologicalSort_Empty(t *testing.T) {
	t.Parallel()

	steps := []Step{}

	order, err := TopologicalSort(steps)
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 0 {
		t.Errorf("expected empty order, got %v", order)
	}
}

// TestBuild_Determinism verifies that building the same plan twice produces identical Order and ConfigHash.
func TestBuild_Determinism(t *testing.T) {
	t.Parallel()

	configData := []byte(`{
  "version": 1,
  "project": {"name": "test"},
  "profiles": {
    "default": {
      "steps": [
        {"id": "z-step", "type": "shell", "command": ["echo", "z"]},
        {"id": "a-step", "type": "shell", "command": ["echo", "a"]},
        {"id": "m-step", "type": "shell", "command": ["echo", "m"]}
      ]
    }
  }
}`)

	steps := []config.Step{
		{ID: "z-step", Type: "shell", Command: []string{"echo", "z"}},
		{ID: "a-step", Type: "shell", Command: []string{"echo", "a"}},
		{ID: "m-step", Type: "shell", Command: []string{"echo", "m"}},
	}

	plan1, err := Build("test-project", "default", steps, configData)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	plan2, err := Build("test-project", "default", steps, configData)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// ConfigHash should be identical.
	if plan1.ConfigHash != plan2.ConfigHash {
		t.Errorf("ConfigHash mismatch: %q vs %q", plan1.ConfigHash, plan2.ConfigHash)
	}

	// Order should be identical (alphabetical: a-step, m-step, z-step).
	if len(plan1.Order) != len(plan2.Order) {
		t.Errorf("Order length mismatch: %d vs %d", len(plan1.Order), len(plan2.Order))
	}

	for i, id := range plan1.Order {
		if id != plan2.Order[i] {
			t.Errorf("Order mismatch at index %d: %q vs %q", i, id, plan2.Order[i])
		}
	}

	// Verify the order is actually sorted (deterministic).
	expectedOrder := []string{"a-step", "m-step", "z-step"}
	for i, id := range expectedOrder {
		if plan1.Order[i] != id {
			t.Errorf("expected step %q at index %d, got %q", id, i, plan1.Order[i])
		}
	}

	// Verify Steps are present and match.
	if len(plan1.Steps) != len(plan2.Steps) {
		t.Errorf("Steps length mismatch: %d vs %d", len(plan1.Steps), len(plan2.Steps))
	}

	// Verify by serializing to JSON and comparing structure (excluding timestamps).
	type PlanSnapshot struct {
		Version     int      `json:"version"`
		ProjectName string   `json:"project_name"`
		Profile     string   `json:"profile"`
		ConfigHash  string   `json:"config_hash"`
		Steps       []Step   `json:"steps"`
		Order       []string `json:"order"`
	}

	snap1 := PlanSnapshot{
		Version:     plan1.Version,
		ProjectName: plan1.ProjectName,
		Profile:     plan1.Profile,
		ConfigHash:  plan1.ConfigHash,
		Steps:       plan1.Steps,
		Order:       plan1.Order,
	}

	snap2 := PlanSnapshot{
		Version:     plan2.Version,
		ProjectName: plan2.ProjectName,
		Profile:     plan2.Profile,
		ConfigHash:  plan2.ConfigHash,
		Steps:       plan2.Steps,
		Order:       plan2.Order,
	}

	b1, _ := json.Marshal(snap1)
	b2, _ := json.Marshal(snap2)

	if string(b1) != string(b2) {
		t.Errorf("Plan snapshots differ:\n%s\nvs\n%s", string(b1), string(b2))
	}
}
