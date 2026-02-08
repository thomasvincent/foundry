// Package config provides configuration management for Foundry CI/CD pipelines.
package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/foundry-ci/foundry/internal/policy"
	"gopkg.in/yaml.v3"
)

// Config represents the complete Foundry configuration loaded from .foundry.yaml.
type Config struct {
	Version  int                `yaml:"version" json:"version"`
	Project  Project            `yaml:"project" json:"project"`
	Policy   policy.Policy      `yaml:"policy" json:"policy"`
	Profiles map[string]Profile `yaml:"profiles" json:"profiles"`
}

// Project represents project-level metadata.
type Project struct {
	Name string `yaml:"name" json:"name"`
}

// Profile represents a named collection of steps that may extend another profile.
type Profile struct {
	Extends string `yaml:"extends,omitempty" json:"extends,omitempty"`
	Steps   []Step `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// Step represents a single execution unit within a profile.
type Step struct {
	ID      string            `yaml:"id" json:"id"`
	Type    string            `yaml:"type" json:"type"`
	Command []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Deps    []string          `yaml:"deps,omitempty" json:"deps,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Timeout string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries int               `yaml:"retries,omitempty" json:"retries,omitempty"`
}

// Load reads and parses a YAML configuration file, then validates the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %q: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses YAML configuration from bytes and validates the result.
// Unknown fields in the YAML cause a parse error.
func LoadFromBytes(data []byte) (*Config, error) {
	cfg := &Config{}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config YAML: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// RawBytes returns the raw YAML bytes for a config file at the given path.
// This is used for config hashing.
func RawBytes(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	return data, nil
}

// Validate checks that the configuration is well-formed and internally consistent.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("validate: config is nil")
	}

	if cfg.Version != 1 {
		return fmt.Errorf("validate: unsupported config version %d (expected 1)", cfg.Version)
	}

	if cfg.Project.Name == "" {
		return fmt.Errorf("validate: project.name must be non-empty")
	}

	if len(cfg.Profiles) == 0 {
		return fmt.Errorf("validate: at least one profile must be defined")
	}

	for profileName, profile := range cfg.Profiles {
		if err := validateProfile(profileName, profile, cfg); err != nil {
			return err
		}
	}

	return nil
}

// validStepTypes lists the allowed step types.
var validStepTypes = []string{"shell", "plugin", "script"}

func validateProfile(name string, profile Profile, cfg *Config) error {
	// Validate extends reference.
	if profile.Extends != "" {
		if _, exists := cfg.Profiles[profile.Extends]; !exists {
			return fmt.Errorf("validate: profile %q extends non-existent profile %q", name, profile.Extends)
		}
	}

	// Check extends cycle.
	visited := map[string]bool{name: true}
	if err := checkExtendsCycle(name, profile, cfg, visited); err != nil {
		return err
	}

	// Validate steps within this profile.
	stepIDs := make(map[string]bool, len(profile.Steps))
	for _, step := range profile.Steps {
		if step.ID == "" {
			return fmt.Errorf("validate: profile %q has step with empty id", name)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("validate: profile %q has duplicate step id %q", name, step.ID)
		}
		stepIDs[step.ID] = true

		if !slices.Contains(validStepTypes, step.Type) {
			return fmt.Errorf("validate: profile %q step %q has invalid type %q (must be shell, plugin, or script)", name, step.ID, step.Type)
		}

		if step.Type == "shell" && len(step.Command) == 0 {
			return fmt.Errorf("validate: profile %q step %q: shell steps must have non-empty command", name, step.ID)
		}

		for _, dep := range step.Deps {
			if !stepIDs[dep] {
				// Dep might reference a step defined before this one; re-check after all steps.
			}
		}
	}

	// Second pass: validate deps reference existing step IDs within this profile.
	for _, step := range profile.Steps {
		for _, dep := range step.Deps {
			if !stepIDs[dep] {
				return fmt.Errorf("validate: profile %q step %q: dependency %q not found in profile", name, step.ID, dep)
			}
		}
	}

	return nil
}

func checkExtendsCycle(origin string, profile Profile, cfg *Config, visited map[string]bool) error {
	if profile.Extends == "" {
		return nil
	}

	if visited[profile.Extends] {
		return fmt.Errorf("validate: profile %q: circular extends chain detected (reaches %q again)", origin, profile.Extends)
	}

	visited[profile.Extends] = true

	parent, exists := cfg.Profiles[profile.Extends]
	if !exists {
		return fmt.Errorf("validate: profile %q extends non-existent profile %q", origin, profile.Extends)
	}

	return checkExtendsCycle(origin, parent, cfg, visited)
}

// ResolveProfile resolves a profile by name, following the extends chain and
// merging steps. Parent steps are inherited; child steps override by ID or are
// appended.
func ResolveProfile(cfg *Config, name string) ([]Step, error) {
	if cfg == nil {
		return nil, fmt.Errorf("resolve profile: config is nil")
	}

	profile, exists := cfg.Profiles[name]
	if !exists {
		return nil, fmt.Errorf("resolve profile: profile %q not found", name)
	}

	visited := map[string]bool{name: true}
	return resolveProfileChain(profile, cfg, visited)
}

func resolveProfileChain(profile Profile, cfg *Config, visited map[string]bool) ([]Step, error) {
	var baseSteps []Step

	if profile.Extends != "" {
		if visited[profile.Extends] {
			return nil, fmt.Errorf("resolve profile: circular extends chain detected")
		}
		visited[profile.Extends] = true

		parent, exists := cfg.Profiles[profile.Extends]
		if !exists {
			return nil, fmt.Errorf("resolve profile: extended profile %q not found", profile.Extends)
		}

		var err error
		baseSteps, err = resolveProfileChain(parent, cfg, visited)
		if err != nil {
			return nil, err
		}
	}

	// Merge current profile's steps onto base.
	for _, step := range profile.Steps {
		replaced := false
		for i, existing := range baseSteps {
			if existing.ID == step.ID {
				baseSteps[i] = step
				replaced = true
				break
			}
		}
		if !replaced {
			baseSteps = append(baseSteps, step)
		}
	}

	return baseSteps, nil
}

// LogConfig logs the loaded configuration at info level for debugging.
func LogConfig(cfg *Config) {
	slog.Info("config loaded",
		"version", cfg.Version,
		"project", cfg.Project.Name,
		"profiles", len(cfg.Profiles),
	)
	for name, p := range cfg.Profiles {
		slog.Info("profile",
			"name", name,
			"extends", p.Extends,
			"steps", len(p.Steps),
		)
	}
}
