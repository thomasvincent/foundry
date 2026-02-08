# CLAUDE.md

Profile-based CI/CD engine with DAG execution model. CLI binary: `anvil`.

## Stack
- Go 1.23.6

## Build & Test
```bash
make build
go test -race -count=1 ./...
golangci-lint run ./...
```

## Notes
- Configuration in `.foundry.yaml` (YAML profiles with step dependencies)
- Output artifacts written to `.foundry/out/` directory
- Step types: shell, plugin, script
- Commands: `anvil doctor`, `anvil plan`, `anvil run`
