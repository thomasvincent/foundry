package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foundry-ci/foundry/internal/plan"
)

// TestExecute_SimpleSuccess verifies that a simple successful shell command executes correctly.
func TestExecute_SimpleSuccess(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "test", Type: "shell", Command: []string{"echo", "hello"}},
		},
		Order: []string{"test"},
	}

	opts := Options{
		Jobs:           1,
		DefaultTimeout: 10 * time.Second,
		FailFast:       true,
		OutDir:         outDir,
	}

	results, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "success" {
		t.Errorf("expected status 'success', got %q", results.Status)
	}

	if len(results.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(results.Steps))
	}

	if results.Steps[0].Status != "success" {
		t.Errorf("expected step status 'success', got %q", results.Steps[0].Status)
	}

	if results.Steps[0].ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", results.Steps[0].ExitCode)
	}
}

// TestExecute_StepFailure verifies that a failing step is recorded as failed.
func TestExecute_StepFailure(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "failing", Type: "shell", Command: []string{"false"}},
		},
		Order: []string{"failing"},
	}

	opts := Options{
		Jobs:           1,
		DefaultTimeout: 10 * time.Second,
		FailFast:       true,
		OutDir:         outDir,
	}

	results, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", results.Status)
	}

	if len(results.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(results.Steps))
	}

	if results.Steps[0].Status != "failed" {
		t.Errorf("expected step status 'failed', got %q", results.Steps[0].Status)
	}

	if results.Steps[0].ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", results.Steps[0].ExitCode)
	}
}

// TestExecute_DependencySkip verifies that steps depending on a failed step are skipped.
func TestExecute_DependencySkip(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "failing", Type: "shell", Command: []string{"false"}},
			{ID: "dependent", Type: "shell", Command: []string{"echo", "dependent"}, Deps: []string{"failing"}},
		},
		Order: []string{"failing", "dependent"},
	}

	opts := Options{
		Jobs:           4,
		DefaultTimeout: 10 * time.Second,
		FailFast:       false,
		OutDir:         outDir,
	}

	results, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "failed" {
		t.Errorf("expected overall status 'failed', got %q", results.Status)
	}

	// Find the dependent step result.
	var depResult *StepResult
	for i := range results.Steps {
		if results.Steps[i].ID == "dependent" {
			depResult = &results.Steps[i]
			break
		}
	}

	if depResult == nil {
		t.Fatal("dependent step result not found")
	}

	if depResult.Status != "skipped" {
		t.Errorf("expected dependent step status 'skipped', got %q", depResult.Status)
	}

	if depResult.Error != "dependency failed" {
		t.Errorf("expected error 'dependency failed', got %q", depResult.Error)
	}
}

// TestExecute_Concurrency verifies that multiple independent steps run concurrently.
func TestExecute_Concurrency(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	// Three independent steps that each sleep 100ms.
	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "a", Type: "shell", Command: []string{"sleep", "0.1"}},
			{ID: "b", Type: "shell", Command: []string{"sleep", "0.1"}},
			{ID: "c", Type: "shell", Command: []string{"sleep", "0.1"}},
		},
		Order: []string{"a", "b", "c"},
	}

	opts := Options{
		Jobs:           3,
		DefaultTimeout: 10 * time.Second,
		FailFast:       true,
		OutDir:         outDir,
	}

	start := time.Now()
	results, err := Execute(context.Background(), p, opts)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "success" {
		t.Errorf("expected status 'success', got %q", results.Status)
	}

	// If running sequentially, it would take ~300ms. With concurrency=3, should be ~100ms.
	// Allow some overhead, but it should be significantly less than 500ms.
	if elapsed >= 500*time.Millisecond {
		t.Errorf("execution took %v, expected < 500ms (suggests insufficient concurrency)", elapsed)
	}
}

// TestExecute_Retries verifies that failed steps are retried and eventually succeed.
func TestExecute_Retries(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	// Create a marker file location in the temp directory.
	markerFile := filepath.Join(outDir, "marker")

	// This command will fail the first time (marker doesn't exist), then succeed.
	// It creates the marker on the first attempt, so the second attempt succeeds.
	cmd := []string{"sh", "-c", "test -f " + markerFile + " || (touch " + markerFile + " && exit 1)"}

	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "retry-test", Type: "shell", Command: cmd, Retries: 1},
		},
		Order: []string{"retry-test"},
	}

	opts := Options{
		Jobs:           1,
		DefaultTimeout: 10 * time.Second,
		FailFast:       true,
		OutDir:         outDir,
	}

	results, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "success" {
		t.Errorf("expected status 'success', got %q", results.Status)
	}

	if len(results.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(results.Steps))
	}

	if results.Steps[0].Status != "success" {
		t.Errorf("expected step status 'success', got %q", results.Steps[0].Status)
	}

	if results.Steps[0].Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", results.Steps[0].Attempt)
	}
}

// TestExecute_LogCapture verifies that step output is captured in log files.
func TestExecute_LogCapture(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	p := &plan.Plan{
		Version:     1,
		ProjectName: "test",
		Profile:     "default",
		ConfigHash:  "abc123",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Steps: []plan.Step{
			{ID: "log-test", Type: "shell", Command: []string{"echo", "hello-from-log"}},
		},
		Order: []string{"log-test"},
	}

	opts := Options{
		Jobs:           1,
		DefaultTimeout: 10 * time.Second,
		FailFast:       true,
		OutDir:         outDir,
	}

	results, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if results.Status != "success" {
		t.Errorf("expected status 'success', got %q", results.Status)
	}

	if len(results.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(results.Steps))
	}

	logPath := results.Steps[0].LogFile
	if logPath == "" {
		t.Fatal("expected log file path in result")
	}

	// Read the log file and verify it contains the expected output.
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", logPath, err)
	}

	if !strings.Contains(string(logContent), "hello-from-log") {
		t.Errorf("expected log to contain 'hello-from-log', got: %q", string(logContent))
	}
}
