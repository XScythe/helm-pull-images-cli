# AGENTS.md

## Project summary

This repository is a Go CLI for rendering Helm charts, extracting referenced images, archiving those images, and pushing them to a target registry.

## Repository layout

- `main.go` boots the Cobra CLI.
- `cmd/` contains the `pull` and `push` commands and their flag wiring.
- `internal/chartmirror/` renders Helm charts and coordinates the pull workflow.
- `internal/chartimages/` extracts image references from rendered manifests.
- `internal/mirror/` handles archive generation, push manifest creation, and push execution.
- `e2e_registry_test.go` covers the registry push path end to end.

## Common commands

- `go test ./...` — run the test suite.
- `go build ./...` — build all packages.
- `go run . --help` — inspect CLI usage.
- `go test ./... -run TestName` — run a focused test when iterating on a single area.

## Working rules

- Keep changes surgical and consistent with the existing cmd/internal split.
- Prefer existing helpers over adding new ones when behavior already exists.
- Keep command code thin; put workflow logic in `internal/` packages.
- Use clear error returns; do not swallow failures.
- Preserve the current Cobra flag and command patterns.
- Add or update tests alongside behavior changes, especially for manifest parsing, archive handling, and registry interactions.

## Notes

- `pull` loads charts in-process with the Helm SDK, resolves remote chart versions from `index.yaml` when needed, and writes archives plus `push_images.json` into the output directory.
- `push` reads `push_images.json` from `--input-dir` or from the helper binary directory by default.
- The CLI expects Helm chart repositories, registry access, and writable local disk at runtime.
- Temporary output is written to the current working directory unless an explicit output directory is provided.
