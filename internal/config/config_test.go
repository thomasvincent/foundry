package config

import (
	"strings"
	"testing"
)

// TestLoadFromBytes_Valid parses valid YAML and verifies all fields are correctly loaded.
func TestLoadFromBytes_Valid(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
policy:
  allow_script_steps: false
profiles:
  default:
    steps:
      - id: lint
        type: shell
        command: ["echo", "lint"]
      - id: test
        type: shell
        deps: ["lint"]
        command: ["echo", "test"]
  ci:
    extends: default
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}

	if cfg.Project.Name != "test-project" {
		t.Errorf("expected project name 'test-project', got %q", cfg.Project.Name)
	}

	if cfg.Policy.AllowScriptSteps != false {
		t.Errorf("expected AllowScriptSteps false, got %v", cfg.Policy.AllowScriptSteps)
	}

	if len(cfg.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(cfg.Profiles))
	}

	defaultProfile, exists := cfg.Profiles["default"]
	if !exists {
		t.Fatal("expected 'default' profile to exist")
	}

	if len(defaultProfile.Steps) != 2 {
		t.Errorf("expected 2 steps in default profile, got %d", len(defaultProfile.Steps))
	}

	if defaultProfile.Steps[0].ID != "lint" {
		t.Errorf("expected first step ID 'lint', got %q", defaultProfile.Steps[0].ID)
	}

	if defaultProfile.Steps[0].Type != "shell" {
		t.Errorf("expected first step type 'shell', got %q", defaultProfile.Steps[0].Type)
	}

	if len(defaultProfile.Steps[0].Command) != 2 || defaultProfile.Steps[0].Command[0] != "echo" {
		t.Errorf("unexpected command for lint step: %v", defaultProfile.Steps[0].Command)
	}

	if defaultProfile.Steps[1].ID != "test" {
		t.Errorf("expected second step ID 'test', got %q", defaultProfile.Steps[1].ID)
	}

	if len(defaultProfile.Steps[1].Deps) != 1 || defaultProfile.Steps[1].Deps[0] != "lint" {
		t.Errorf("expected second step to depend on 'lint', got %v", defaultProfile.Steps[1].Deps)
	}

	ciProfile, exists := cfg.Profiles["ci"]
	if !exists {
		t.Fatal("expected 'ci' profile to exist")
	}

	if ciProfile.Extends != "default" {
		t.Errorf("expected 'ci' profile to extend 'default', got %q", ciProfile.Extends)
	}
}

// TestLoadFromBytes_UnknownFields verifies that unknown fields in YAML cause an error.
func TestLoadFromBytes_UnknownFields(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
unknown_field: true
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

// TestLoadFromBytes_InvalidVersion verifies that unsupported versions are rejected.
func TestLoadFromBytes_InvalidVersion(t *testing.T) {
	t.Parallel()

	yaml := `
version: 2
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for version 2, got nil")
	}

	if err.Error() != "validate: unsupported config version 2 (expected 1)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadFromBytes_EmptyProjectName verifies that empty project names are rejected.
func TestLoadFromBytes_EmptyProjectName(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: ""
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty project name, got nil")
	}

	if err.Error() != "validate: project.name must be non-empty" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadFromBytes_DuplicateStepID verifies that duplicate step IDs are rejected.
func TestLoadFromBytes_DuplicateStepID(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
      - id: test
        type: shell
        command: ["echo", "test2"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate step ID, got nil")
	}

	if err.Error() != "validate: profile \"default\" has duplicate step id \"test\"" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadFromBytes_InvalidStepType verifies that invalid step types are rejected.
func TestLoadFromBytes_InvalidStepType(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: invalid
        command: ["echo", "test"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid step type, got nil")
	}

	if err.Error() != "validate: profile \"default\" step \"test\" has invalid type \"invalid\" (must be shell, plugin, or script)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadFromBytes_ShellNoCommand verifies that shell steps without a command are rejected.
func TestLoadFromBytes_ShellNoCommand(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: shell
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for shell step without command, got nil")
	}

	if err.Error() != "validate: profile \"default\" step \"test\": shell steps must have non-empty command" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadFromBytes_InvalidDep verifies that steps depending on non-existent IDs are rejected.
func TestLoadFromBytes_InvalidDep(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
        deps: ["nonexistent"]
`

	_, err := LoadFromBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid dependency, got nil")
	}

	if err.Error() != "validate: profile \"default\" step \"test\": dependency \"nonexistent\" not found in profile" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestResolveProfile_Simple verifies that a simple profile without extends is resolved correctly.
func TestResolveProfile_Simple(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: lint
        type: shell
        command: ["echo", "lint"]
      - id: test
        type: shell
        command: ["echo", "test"]
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	steps, err := ResolveProfile(cfg, "default")
	if err != nil {
		t.Fatalf("ResolveProfile failed: %v", err)
	}

	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}

	if steps[0].ID != "lint" || steps[1].ID != "test" {
		t.Errorf("unexpected step order: %v", []string{steps[0].ID, steps[1].ID})
	}
}

// TestResolveProfile_Extends verifies that a profile that extends another merges steps correctly.
func TestResolveProfile_Extends(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: lint
        type: shell
        command: ["echo", "lint"]
      - id: test
        type: shell
        command: ["echo", "test"]
  ci:
    extends: default
    steps:
      - id: build
        type: shell
        command: ["echo", "build"]
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	steps, err := ResolveProfile(cfg, "ci")
	if err != nil {
		t.Fatalf("ResolveProfile failed: %v", err)
	}

	if len(steps) != 3 {
		t.Errorf("expected 3 steps (inherited + new), got %d", len(steps))
	}

	// Verify all expected step IDs are present.
	ids := map[string]bool{}
	for _, s := range steps {
		ids[s.ID] = true
	}

	if !ids["lint"] || !ids["test"] || !ids["build"] {
		t.Errorf("expected steps lint, test, build; got %v", ids)
	}
}

// TestResolveProfile_CycleDetection verifies that circular extends chains are detected.
func TestResolveProfile_CycleDetection(t *testing.T) {
	t.Parallel()

	// Note: We can't create a truly circular YAML in one pass, so we'll test by
	// manually constructing a Config with a cycle and validating it's caught.
	cfg := &Config{
		Version: 1,
		Project: Project{Name: "test"},
		Profiles: map[string]Profile{
			"a": {
				Extends: "b",
				Steps: []Step{
					{ID: "step-a", Type: "shell", Command: []string{"echo", "a"}},
				},
			},
			"b": {
				Extends: "a",
				Steps: []Step{
					{ID: "step-b", Type: "shell", Command: []string{"echo", "b"}},
				},
			},
		},
	}

	// Validation should catch the cycle.
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for circular extends, got nil")
	}

	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular error, got: %v", err)
	}
}

// TestResolveProfile_NotFound verifies that resolving a non-existent profile fails.
func TestResolveProfile_NotFound(t *testing.T) {
	t.Parallel()

	yaml := `
version: 1
project:
  name: "test-project"
profiles:
  default:
    steps:
      - id: test
        type: shell
        command: ["echo", "test"]
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	_, err = ResolveProfile(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent profile, got nil")
	}

	if err.Error() != "resolve profile: profile \"nonexistent\" not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}
