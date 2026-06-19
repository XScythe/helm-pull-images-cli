# AGENTS.md

## Project summary

This repository is a Go CLI for mirroring Helm chart images into archives and generating a push manifest.

## Repository layout

- `main.go` boots the Cobra CLI.
- `cmd/` contains CLI commands.
- `internal/chartmirror/` renders Helm charts and coordinates the mirroring flow.
- `internal/chartimages/` extracts image references from manifests.
- `internal/mirror/` handles archive generation, push manifest creation, and push execution.

## Common commands

- `go test ./...` — run the test suite.
- `go build ./...` — build all packages.
- `go run . --help` — inspect CLI usage.

## Working rules

- Keep changes surgical and consistent with existing package boundaries.
- Prefer existing helpers over adding new ones when behavior already exists.
- Use clear error returns; do not swallow failures.
- Preserve the current Cobra flag and command patterns.
- Add or update tests alongside behavior changes.

## Notes

- The CLI expects Helm-related tooling and registry access at runtime.
- Temporary output is written to the current working directory unless an explicit output directory is provided.
