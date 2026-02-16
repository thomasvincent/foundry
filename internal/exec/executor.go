// Package exec provides execution logic for Foundry CI/CD plans.
package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/foundry-ci/foundry/internal/plan"
)

// Options configures execution behavior.
type Options struct {
	OutDir         string        // Directory for output logs
	DefaultTimeout time.Duration // Default timeout for steps without explicit timeout
	Jobs           int           // Number of concurrent jobs
	FailFast       bool          // Stop execution on first failure
}

// StepResult represents the result of executing a single step.
type StepResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // success, failed, skipped
	Error    string `json:"error,omitempty"`
	LogFile  string `json:"log_file,omitempty"`
	Duration string `json:"duration"`
	ExitCode int    `json:"exit_code"`
	Attempt  int    `json:"attempt"` // Number of attempts made (1-indexed)
}

// ExecutionResult represents the overall result of executing a plan.
type ExecutionResult struct {
	Status   string       `json:"status"`
	Duration string       `json:"duration"`
	Steps    []StepResult `json:"steps"`
}

// Execute runs the given plan according to the specified options.
func Execute(ctx context.Context, p *plan.Plan, opts Options) (*ExecutionResult, error) {
	if p == nil {
		return nil, fmt.Errorf("execute: plan is nil")
	}

	startTime := time.Now()

	// Create output directory if needed.
	if opts.OutDir != "" {
		if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
			return nil, fmt.Errorf("execute: create output directory: %w", err)
		}
	}

	// Build step lookup map.
	stepMap := make(map[string]plan.Step, len(p.Steps))
	for _, step := range p.Steps {
		stepMap[step.ID] = step
	}

	// Track step results and completion status.
	results := make(map[string]*StepResult, len(p.Steps))
	resultsMu := sync.Mutex{}

	// Semaphore for concurrency control.
	sem := make(chan struct{}, opts.Jobs)

	// Track failed steps for dependency skipping.
	failedSteps := make(map[string]bool)
	failedMu := sync.Mutex{}

	// Context for fail-fast cancellation.
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Execute steps in order, respecting dependencies.
	var wg sync.WaitGroup
	for _, stepID := range p.Order {
		stepID := stepID
		step, exists := stepMap[stepID]
		if !exists {
			return nil, fmt.Errorf("execute: step %q in order but not in steps", stepID)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-execCtx.Done():
				resultsMu.Lock()
				results[stepID] = &StepResult{
					ID:       stepID,
					Status:   "skipped",
					Error:    "execution cancelled",
					Attempt:  0,
					Duration: "0s",
				}
				resultsMu.Unlock()
				return
			}

			// Wait for dependencies to complete.
			for {
				allDepsComplete := true
				resultsMu.Lock()
				for _, dep := range step.Deps {
					if _, complete := results[dep]; !complete {
						allDepsComplete = false
						break
					}
				}
				resultsMu.Unlock()

				if allDepsComplete {
					break
				}

				// Sleep briefly before checking again.
				select {
				case <-time.After(10 * time.Millisecond):
				case <-execCtx.Done():
					resultsMu.Lock()
					results[stepID] = &StepResult{
						ID:       stepID,
						Status:   "skipped",
						Error:    "execution cancelled",
						Attempt:  0,
						Duration: "0s",
					}
					resultsMu.Unlock()
					return
				}
			}

			// Check if dependencies failed (after they completed).
			failedMu.Lock()
			depFailed := false
			for _, dep := range step.Deps {
				if failedSteps[dep] {
					depFailed = true
					break
				}
			}
			failedMu.Unlock()

			if depFailed {
				resultsMu.Lock()
				results[stepID] = &StepResult{
					ID:       stepID,
					Status:   "skipped",
					Error:    "dependency failed",
					Attempt:  0,
					Duration: "0s",
				}
				resultsMu.Unlock()
				return
			}

			// Execute the step.
			result := executeStep(execCtx, step, opts)

			resultsMu.Lock()
			results[stepID] = result
			resultsMu.Unlock()

			// Track failure for dependency skipping.
			if result.Status == "failed" {
				failedMu.Lock()
				failedSteps[stepID] = true
				failedMu.Unlock()

				// Cancel execution if fail-fast is enabled.
				if opts.FailFast {
					cancel()
				}
			}
		}()
	}

	wg.Wait()

	// Collect results in order.
	var stepResults []StepResult
	overallStatus := "success"
	for _, stepID := range p.Order {
		result, exists := results[stepID]
		if !exists {
			return nil, fmt.Errorf("execute: missing result for step %q", stepID)
		}
		stepResults = append(stepResults, *result)
		if result.Status == "failed" {
			overallStatus = "failed"
		}
	}

	duration := time.Since(startTime)

	return &ExecutionResult{
		Status:   overallStatus,
		Steps:    stepResults,
		Duration: duration.String(),
	}, nil
}

// executeStep executes a single step with retries.
func executeStep(ctx context.Context, step plan.Step, opts Options) *StepResult {
	maxAttempts := step.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastResult *StepResult
	stepStart := time.Now()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptStart := time.Now()
		result := executeStepAttempt(ctx, step, opts, attempt)
		result.Duration = time.Since(attemptStart).String()

		if result.Status == "success" {
			result.Duration = time.Since(stepStart).String()
			return result
		}

		lastResult = result

		// Don't retry if context is cancelled.
		if ctx.Err() != nil {
			break
		}

		// Brief delay before retry.
		if attempt < maxAttempts {
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				break
			}
		}
	}

	lastResult.Duration = time.Since(stepStart).String()
	return lastResult
}

// CheckTool checks if a tool is available by running it with the given argument.
func CheckTool(toolName, arg string) error {
	cmd := exec.Command(toolName, arg)
	return cmd.Run()
}

// DefaultOptions returns default execution options.
func DefaultOptions() Options {
	return Options{
		Jobs:           4,
		DefaultTimeout: 5 * time.Minute,
		FailFast:       true,
		OutDir:         ".foundry/out",
	}
}

// WriteResults writes execution results to a JSON file in the output directory.
func WriteResults(results *ExecutionResult, outDir string) error {
	if results == nil {
		return fmt.Errorf("write results: results is nil")
	}

	if outDir == "" {
		return fmt.Errorf("write results: output directory is empty")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("write results: create output directory: %w", err)
	}

	resultsPath := filepath.Join(outDir, "results.json")
	data, err := os.Create(resultsPath)
	if err != nil {
		return fmt.Errorf("write results: create file: %w", err)
	}
	defer func() { _ = data.Close() }()

	encoder := json.NewEncoder(data)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("write results: encode JSON: %w", err)
	}

	return nil
}

// executeStepAttempt executes a single attempt of a step.
func executeStepAttempt(ctx context.Context, step plan.Step, opts Options, attempt int) *StepResult {
	result := &StepResult{
		ID:      step.ID,
		Status:  "failed",
		Attempt: attempt,
	}

	// Only shell type is supported currently.
	if step.Type != "shell" {
		result.Error = fmt.Sprintf("unsupported step type: %s", step.Type)
		return result
	}

	if len(step.Command) == 0 {
		result.Error = "empty command"
		return result
	}

	// Create log file.
	var logFile *os.File
	var logPath string
	if opts.OutDir != "" {
		logFileName := fmt.Sprintf("%s.%d.log", step.ID, attempt)
		logPath = filepath.Join(opts.OutDir, logFileName)
		var err error
		logFile, err = os.Create(logPath)
		if err != nil {
			result.Error = fmt.Sprintf("create log file: %v", err)
			return result
		}
		defer func() { _ = logFile.Close() }()
		result.LogFile = logPath
	}

	// Build command.
	cmd := exec.CommandContext(ctx, step.Command[0], step.Command[1:]...)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	// Set environment.
	if len(step.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range step.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Apply timeout.
	timeout := opts.DefaultTimeout
	if step.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(step.Timeout)
		if err != nil {
			result.Error = fmt.Sprintf("invalid timeout: %v", err)
			return result
		}
		timeout = parsedTimeout
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, step.Command[0], step.Command[1:]...)
		if logFile != nil {
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
		if len(step.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range step.Env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// Execute command.
	slog.Info("executing step", "id", step.ID, "attempt", attempt, "command", step.Command)
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Error = err.Error()
		return result
	}

	result.Status = "success"
	result.ExitCode = 0
	return result
}
