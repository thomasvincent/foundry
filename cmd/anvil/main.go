// Package main implements the anvil CLI for the Foundry CI/CD engine.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/foundry-ci/foundry/internal/config"
	"github.com/foundry-ci/foundry/internal/exec"
	"github.com/foundry-ci/foundry/internal/plan"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "--version":
		cmdVersion(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "plan":
		cmdPlan(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Foundry — CI/CD engine

Usage: anvil <command> [flags]

Commands:
  version    Print version information
  doctor     Check environment and configuration
  plan       Generate an execution plan
  run        Execute the plan

Use "anvil <command> --help" for more information.
`)
}

func setupLogger(jsonOutput bool) {
	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))
}

// --- version ---

func cmdVersion(args []string) {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]string{
			"version":    version,
			"commit":     commit,
			"build_date": buildDate,
		})
	} else {
		fmt.Printf("anvil %s (commit %s, built %s)\n", version, commit, buildDate)
	}
}

// --- doctor ---

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configPath := fs.String("config", ".foundry.yaml", "config file path")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	setupLogger(false)
	allPass := true

	// Check 1: config file exists.
	if _, err := os.Stat(*configPath); err != nil {
		fmt.Printf("FAIL  %s not found\n", *configPath)
		allPass = false
	} else {
		fmt.Printf("PASS  %s exists\n", *configPath)
	}

	// Check 2: config parses and validates.
	if _, err := config.Load(*configPath); err != nil {
		fmt.Printf("FAIL  config validation: %v\n", err)
		allPass = false
	} else {
		fmt.Printf("PASS  config parses and validates\n")
	}

	// Check 3: go is available.
	if err := exec.CheckTool("go", "version"); err != nil {
		fmt.Printf("FAIL  go not available: %v\n", err)
		allPass = false
	} else {
		fmt.Printf("PASS  go is available\n")
	}

	if !allPass {
		os.Exit(1)
	}
	fmt.Println("\nAll checks passed.")
}

// --- plan ---

func cmdPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	profileName := fs.String("profile", "default", "profile name")
	configPath := fs.String("config", ".foundry.yaml", "config file path")
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	setupLogger(*jsonOut)

	cfg, steps, configData := loadAndResolve(*configPath, *profileName)

	// Validate steps against policy.
	for _, s := range steps {
		if err := cfg.Policy.ValidateStep(s.Type, s.ID); err != nil {
			slog.Error("policy violation", "error", err)
			os.Exit(1)
		}
	}

	p, err := plan.Build(cfg.Project.Name, *profileName, steps, configData)
	if err != nil {
		slog.Error("failed to build plan", "error", err)
		os.Exit(1)
	}

	outDir := ".foundry/out"
	if err := plan.WritePlan(p, outDir); err != nil {
		slog.Error("failed to write plan", "error", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(p)
	} else {
		fmt.Printf("Plan generated: %d steps, profile=%s\n", len(p.Steps), *profileName)
		fmt.Println("Execution order:")
		for i, id := range p.Order {
			fmt.Printf("  %d. %s\n", i+1, id)
		}
		fmt.Println("Written to .foundry/out/plan.json")
	}
}

// --- run ---

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	profileName := fs.String("profile", "default", "profile name")
	configPath := fs.String("config", ".foundry.yaml", "config file path")
	jobs := fs.Int("jobs", 4, "max parallel jobs")
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	setupLogger(*jsonOut)

	cfg, steps, configData := loadAndResolve(*configPath, *profileName)

	for _, s := range steps {
		if err := cfg.Policy.ValidateStep(s.Type, s.ID); err != nil {
			slog.Error("policy violation", "error", err)
			os.Exit(1)
		}
	}

	p, err := plan.Build(cfg.Project.Name, *profileName, steps, configData)
	if err != nil {
		slog.Error("failed to build plan", "error", err)
		os.Exit(1)
	}

	outDir := ".foundry/out"
	if err := plan.WritePlan(p, outDir); err != nil {
		slog.Error("failed to write plan", "error", err)
		os.Exit(1)
	}

	// Execute with signal handling.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	opts := exec.DefaultOptions()
	opts.Jobs = *jobs
	opts.OutDir = outDir

	results, err := exec.Execute(ctx, p, opts)
	if err != nil {
		slog.Error("execution failed", "error", err)
		os.Exit(1)
	}

	if err := exec.WriteResults(results, outDir); err != nil {
		slog.Error("failed to write results", "error", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	} else {
		fmt.Printf("\nExecution %s (%s)\n", results.Status, results.Duration)
		for _, sr := range results.Steps {
			marker := "✓"
			if sr.Status != "success" {
				marker = "✗"
			}
			fmt.Printf("  %s %s [%s] %s\n", marker, sr.ID, sr.Status, sr.Duration)
		}
	}

	if results.Status != "success" {
		os.Exit(1)
	}
}

// --- helpers ---

// loadAndResolve loads config, resolves the profile, and returns raw config bytes.
func loadAndResolve(configPath, profileName string) (*config.Config, []config.Step, []byte) {
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "path", configPath, "error", err)
		os.Exit(1)
	}

	steps, err := config.ResolveProfile(cfg, profileName)
	if err != nil {
		slog.Error("failed to resolve profile", "profile", profileName, "error", err)
		os.Exit(1)
	}

	configData, err := config.RawBytes(configPath)
	if err != nil {
		slog.Error("failed to read config bytes", "error", err)
		os.Exit(1)
	}

	return cfg, steps, configData
}
