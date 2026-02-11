// Package plan provides plan building and topological sorting for Foundry CI/CD pipelines.
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/foundry-ci/foundry/internal/config"
)

// Plan represents an execution plan for a Foundry profile.
type Plan struct {
	Version     int        `json:"version"`
	ProjectName string     `json:"project_name"`
	Profile     string     `json:"profile"`
	ConfigHash  string     `json:"config_hash"`
	CreatedAt   string     `json:"created_at"`
	Steps       []PlanStep `json:"steps"`
	Order       []string   `json:"order"`
}

// PlanStep represents a step within an execution plan.
type PlanStep struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Command []string          `json:"command,omitempty"`
	Deps    []string          `json:"deps,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
	Retries int               `json:"retries,omitempty"`
}

// Build creates an execution plan from resolved configuration steps.
func Build(projectName, profileName string, steps []config.Step, configData []byte) (*Plan, error) {
	if projectName == "" {
		return nil, fmt.Errorf("build plan: project name is empty")
	}
	if profileName == "" {
		return nil, fmt.Errorf("build plan: profile name is empty")
	}

	// Convert config.Step to PlanStep.
	planSteps := make([]PlanStep, len(steps))
	for i, s := range steps {
		planSteps[i] = PlanStep{
			ID:      s.ID,
			Type:    s.Type,
			Command: s.Command,
			Deps:    s.Deps,
			Env:     s.Env,
			Timeout: s.Timeout,
			Retries: s.Retries,
		}
	}

	// Compute topological order.
	order, err := TopologicalSort(planSteps)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	// Compute config hash.
	hash := sha256.Sum256(configData)
	configHash := hex.EncodeToString(hash[:])

	return &Plan{
		Version:     1,
		ProjectName: projectName,
		Profile:     profileName,
		ConfigHash:  configHash,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps:       planSteps,
		Order:       order,
	}, nil
}

// TopologicalSort produces a deterministic execution order for plan steps.
// Steps with no dependencies are sorted alphabetically for determinism.
// Returns an error if a cycle is detected.
func TopologicalSort(steps []PlanStep) ([]string, error) {
	if len(steps) == 0 {
		return []string{}, nil
	}

	// Build adjacency list and in-degree map.
	stepMap := make(map[string]PlanStep, len(steps))
	inDegree := make(map[string]int, len(steps))
	adjList := make(map[string][]string, len(steps))

	for _, step := range steps {
		stepMap[step.ID] = step
		if _, exists := inDegree[step.ID]; !exists {
			inDegree[step.ID] = 0
		}
		for _, dep := range step.Deps {
			adjList[dep] = append(adjList[dep], step.ID)
			inDegree[step.ID]++
		}
	}

	// Collect nodes with zero in-degree (sorted alphabetically for determinism).
	var ready []string
	for id := range stepMap {
		if inDegree[id] == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready)

	var order []string
	for len(ready) > 0 {
		// Pop the first element (alphabetically smallest).
		current := ready[0]
		ready = ready[1:]
		order = append(order, current)

		// Reduce in-degree for neighbors.
		neighbors := adjList[current]
		slices.Sort(neighbors) // Sort for deterministic tie-breaking.
		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				ready = append(ready, neighbor)
				slices.Sort(ready) // Keep ready list sorted.
			}
		}
	}

	// Check for cycles.
	if len(order) != len(steps) {
		return nil, fmt.Errorf("topological sort: cycle detected (only %d of %d steps ordered)", len(order), len(steps))
	}

	return order, nil
}

// WritePlan writes the plan to a JSON file in the output directory.
func WritePlan(p *Plan, outDir string) error {
	if p == nil {
		return fmt.Errorf("write plan: plan is nil")
	}

	if outDir == "" {
		return fmt.Errorf("write plan: output directory is empty")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("write plan: create output directory: %w", err)
	}

	planPath := filepath.Join(outDir, "plan.json")
	data, err := os.Create(planPath)
	if err != nil {
		return fmt.Errorf("write plan: create file: %w", err)
	}
	defer data.Close()

	encoder := json.NewEncoder(data)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(p); err != nil {
		return fmt.Errorf("write plan: encode JSON: %w", err)
	}

	return nil
}
