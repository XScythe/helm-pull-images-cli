# AGENTS.md

## Project

Go CLI for rendering Helm charts, extracting referenced container images, archiving
them into an OCI layout, and pushing them to a target registry.

## Architecture

The codebase is organized as a thin command layer over internal packages grouped by
the two lifecycle phases the tool supports: **pull** (render + stage) and **push**
(transfer to a registry).

**Thin Command Layer** (`cmd/`)
- Parse flags → validate (`PreRunE`) → delegate to an internal package.
- `cmd/pull.go` builds `pull.Options` and calls `pull.Run(...)`.
- `cmd/push.go` calls `push.PushImages(...)`.
- `cmd/root.go` wires subcommands and exposes `commandLogger(verbose bool) *slog.Logger`,
  a small helper that returns a stderr `slog` logger at Debug level when `--verbose`
  is set, otherwise Info. There is no central Config/DI struct.

**Internal Layer** (`internal/`)
- `internal/pull/` — Pull workflow: renders Helm charts in-process via the Helm SDK,
  resolves remote chart versions from `index.yaml`, coordinates image extraction, and
  orchestrates staging (archives + `push_images.json`) into the output directory.
  - `runner.go` — public API (`Run`, `Runner`, `NewRunner`, `Options`, `PullResult`).
  - `pipeline.go`, `render.go`, `chartsource.go`, `repoindex.go` — workflow stages.
  - `outputdir.go`, `slices.go` — small focused helpers.
  - `Runner`'s function-field collaborators are deliberate **test seams**: each one is
    substituted in `runner_test.go` to drive the workflow without Helm/registry/network.
    Keep them as fields — don't collapse them into direct calls (they are real seams,
    not hypothetical ones).
- `internal/push/` — Transfer engine: stages images into an OCI layout
  (`ArchiveImages`) and pushes them to a registry with bounded concurrency
  (`PushImages`), plus progress reporting and self-binary copy helpers.
  - `archive.go`, `push.go`, `files.go`, `progress.go`.
- `internal/pushspec/` — Shared on-disk contract between pull and push: the
  `push_images.json` manifest model and image-reference derivation.
  - `manifest.go` — `ArchiveSpec`, `PushManifest`, read/write helpers,
    `PushManifestFileName`, `OCILayoutDirName`.
  - `spec.go` — `BuildSpecs` and mirror-reference derivation.
- `internal/chartimages/` — Extracts image references from rendered manifests.
  - `extract.go`, `manifest.go`, `annotations.go`, `args.go`, `reference.go`.
- `internal/validation/` — Reusable validators for flags and inputs.

Package dependency direction (no cycles): `pull` → `push`, `pushspec`;
`push` → `pushspec`; `pushspec` → stdlib + go-containerregistry `name`.

## Best Practices & Patterns

### Input Validation
- **Required validation**: Use Cobra's `MarkFlagRequired()` in cmd files.
- **Format/constraint validation**: Use validators in `internal/validation/` from `PreRunE`.
- **Delegate to underlying libraries**: Never use regex; use the actual library validators:
  - Chart names: `chartutil.ValidateMetadataName()` from the Helm SDK
  - Release names: `chartutil.ValidateReleaseName()` from the Helm SDK
  - Namespaces: `validation.IsDNS1123Subdomain()` from Kubernetes apimachinery
  - URLs: Go's standard `url.Parse()`
- Separation of concerns: Cobra handles "is it provided", validators handle "is it valid".

### Logging
- Use `commandLogger(verbose)` in `cmd/` for the small amount of structured logging
  the CLI emits. It returns a stderr `slog.Logger`. Keep logging at command boundaries.
- Never log secrets.

### Status Reporting
- `pull.Run`, `push.ArchiveImages`, and `push.PushImages` accept optional
  `io.Writer` status arguments for human-readable progress; keep this pattern.

### Table-Driven Tests
- Prefer data-driven test cases for breadth with low boilerplate.

## Working Rules

**Commands**
- Use `MarkFlagRequired()` for required flags (Cobra handles presence validation).
- Use `PreRunE()` for format/constraint validation via `internal/validation/`.
- Keep commands thin: validate → delegate. No workflow logic in commands.

**Internal Packages**
- Keep the cmd/internal split and the pull/push/pushspec boundaries intact.
- Put workflow logic in `internal/`; prefer existing helpers over new ones.
- Each package should stay independently testable. No global state or singletons.

**Error Handling**
- Return clear, wrapped errors (`fmt.Errorf("...: %w", err)`). Never swallow failures.

**Testing**
- Add or update tests alongside behavior changes, especially for manifest parsing,
  archive handling, and registry interactions.
- Keep unit tests in-package with the `*_test.go` suffix.

**Code Changes**
- Make surgical changes consistent with existing patterns.
- Preserve the current Cobra flag and command patterns.

## cmd/ Testing

Helpers live in `cmd/shared_test.go`:
- `ExecuteCommand(cmd, args)` → run a command, capture output.
- `AssertFlagExists` / `AssertFlagNotExists` / `AssertFlagType` / `AssertFlagDefault`
  / `AssertFlagRequired` → verify flag registration and properties.
- `PatchCobraRunE(cmd, mockFunc)` → mock execution with automatic state reset.

Coverage spans `cmd/pull_test.go`, `cmd/push_test.go`, and `cmd/root_test.go`:
flag registration, types, defaults, required status, validation rules, and
execution with valid inputs.

```go
restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
    return nil // mock backend
})
defer restore()
```

## Commands

- `go test ./...` — Run tests
- `go build ./...` — Build
- `go vet ./...` — Vet
- `go run . --help` — Help
- `go run . pull --chart nginx --verbose` — Pull with verbose logging
- `go test ./... -run TestName` — Run a focused test

## Notes

- `pull` loads charts in-process via the Helm SDK, resolves remote versions from
  `index.yaml` when needed, and writes archives plus `push_images.json` into the
  output directory.
- `push` reads `push_images.json` from `--input-dir` (or the helper binary directory
  by default), then pushes images with bounded concurrency.
- The on-disk contract (`push_images.json`, OCI layout) lives in `internal/pushspec/`
  and is shared by both phases.
- Temporary output is written to the current working directory unless an explicit
  output directory is provided.
- `e2e_registry_test.go` (repo root) covers the registry push path end to end.
