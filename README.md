# Foundry

A deterministic, profile-based CI/CD engine.

## Quickstart

### Install

```bash
go install ./cmd/anvil
```

### Create Configuration

Create a `.foundry.yaml` file in your project root:

```yaml
version: 1
project:
  name: "my-project"
profiles:
  default:
    steps:
      - id: lint
        type: shell
        command: ["bash", "-lc", "go fmt ./..."]
      - id: test
        type: shell
        deps: ["lint"]
        command: ["bash", "-lc", "go test ./..."]
```

### Run Commands

Check your environment:

```bash
anvil doctor
```

Preview the execution plan:

```bash
anvil plan --profile default
```

Execute the pipeline:

```bash
anvil run --profile default
```

## Configuration

Foundry uses YAML-based configuration files (`.foundry.yaml`) to define CI/CD pipelines. The configuration schema includes:

- **version**: Configuration schema version (currently 1)
- **project**: Project metadata with a required `name` field
- **policy**: Policy settings (e.g., `allow_script_steps` boolean)
- **profiles**: Named execution profiles containing ordered steps

Each step specifies:
- `id`: Unique identifier
- `type`: `shell`, `plugin`, or `script`
- `command`: Command and arguments to execute
- `deps`: Optional list of step IDs this step depends on
- `env`: Optional environment variables
- `timeout`: Optional execution timeout
- `retries`: Optional retry count (0 or more)

Profiles can extend other profiles using the `extends` field.

## CLI Reference

### anvil doctor

Validates the local environment and configuration. Checks for required tools, permissions, and schema compliance.

```bash
anvil doctor
```

### anvil plan

Displays the execution plan for a given profile without running it.

```bash
anvil plan --profile <profile-name>
```

Flags:
- `--profile`: Profile name to plan (required)
- `--verbose`: Show detailed step information

### anvil run

Executes the specified profile.

```bash
anvil run --profile <profile-name>
```

Flags:
- `--profile`: Profile name to run (required)
- `--verbose`: Show detailed execution output
- `--dry-run`: Show what would be executed without running

### anvil version

Displays version information.

```bash
anvil version
```

## Output Structure

Foundry generates execution output in the `.foundry/out/` directory:

```
.foundry/
├── out/
│   ├── <profile-name>/
│   │   ├── <step-id>.log
│   │   ├── <step-id>.json
│   │   └── metadata.json
│   └── ...
└── cache/
```

Each step produces:
- `.log`: Raw text output
- `.json`: Structured execution result with timing and status

## Development

Build the binary:

```bash
make build
```

Run tests:

```bash
make test
```

Run linters:

```bash
make lint
```

Run all checks and build:

```bash
make all
```

Additional targets:

- `make install`: Install binary globally
- `make fmt`: Format code
- `make vet`: Run Go vet
- `make clean`: Remove build artifacts

## Contributing

Contributions are welcome. Please follow these guidelines:

- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages
- Run `make lint` and `make test` before submitting a PR
- Update documentation for new features
- Write tests for bug fixes and features

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE) for details.
