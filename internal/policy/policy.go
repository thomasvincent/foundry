// Package policy provides policy enforcement for Foundry projects.
package policy

import (
	"fmt"
)

// Policy represents the policy configuration for a Foundry project.
type Policy struct {
	AllowScriptSteps bool `yaml:"allow_script_steps" json:"allow_script_steps"`
}

// DefaultPolicy returns a Policy with secure defaults (all restrictive).
func DefaultPolicy() Policy {
	return Policy{
		AllowScriptSteps: false,
	}
}

// ValidateStep checks that a step is allowed under the given policy.
// It returns an error if the step violates policy.
// For v0.1: if step type is "script" and AllowScriptSteps is false, return error.
func (p Policy) ValidateStep(stepType string, stepID string) error {
	if stepType == "script" && !p.AllowScriptSteps {
		return fmt.Errorf("step %q: script steps are not allowed by policy", stepID)
	}
	return nil
}
