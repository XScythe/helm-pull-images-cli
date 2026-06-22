# AGENTS.md

## Project

Go CLI for rendering Helm charts, extracting images, archiving them, and pushing to registries.

## Architecture

**Thin Command Layer** (`cmd/`)
- Parse flags → validate → create config → delegate to internal packages
- Example: `cmd/pull.go` validates inputs, creates Config, calls `chartmirror.Run()`

**Thick Internal Layer** (`internal/`)
- All business logic organized by concern
- Core packages: `log`, `config`, `validation`, `errors`, `chartmirror`, `chartimages`, `mirror`

## Best Practices & Patterns

### 1. Dependency Injection
- Central `Config` struct holds all dependencies (Logger, HTTPClient, flags)
- Pass `*Config` to all major functions for consistency and testability
- Fluent builder: `config.New().WithVerbose(true).WithDebug(debug)`
- Trivial to mock for tests

### 2. Structured Logging
- Use `cfg.Logger.Info()`, `Debug()`, `Warn()`, `Error()` with key-value pairs
- Log at operation boundaries: starts, external calls, errors
- Never log secrets

### 3. Input Validation
- **Required validation**: Use Cobra's `MarkFlagRequired()` in cmd files
- **Format/constraint validation**: Use custom validators in `internal/validation/` called from `PreRunE`
- **Delegate to underlying libraries**: Never use regex; use the actual library validators:
  - Chart names: `chartutil.ValidateMetadataName()` from Helm SDK
  - Release names: `chartutil.ValidateReleaseName()` from Helm SDK
  - Namespaces: `validation.IsDNS1123Subdomain()` from Kubernetes apimachinery
  - URLs: Go's standard `url.Parse()`
- Separation of concerns: Cobra handles "is it provided", validators handle "is it valid"
- Example: `--chart` is required (Cobra), validated by Helm SDK (ValidateChartName in validation.go)

### 4. Error Handling
- Typed errors in `internal/errors/` for classification
- Types: ValidationError, ConfigError, NetworkError, ResourceError, ExecutionError, InternalError
- Wrap with context: `errors.Wrap(NetworkError, "fetch failed", err).WithContext("url", url)`
- Check types: `if errors.IsType(err, NetworkError) { retry() }`

### 5. Builder Pattern
- Fluent method chaining for configuration
- Methods return `*Config` for chaining

### 6. Table-Driven Tests
- Data-driven test cases reduce boilerplate
- Comprehensive coverage across scenarios

## Working Rules

**Commands**
- Use `MarkFlagRequired()` for required flags (Cobra handles presence validation)
- Use `PreRunE()` hook for format/constraint validation via `internal/validation/` package
- Keep focused: validate → create config → delegate
- No workflow logic in commands

**Validation Strategy**
- Cobra layer: `MarkFlagRequired()` for presence checks
- Custom layer: `internal/validation/` for format and constraint checks
- **Delegate to underlying libraries**: Use Helm SDK validators (ValidateMetadataName, ValidateReleaseName), Kubernetes validators (IsDNS1123Subdomain), and Go stdlib (url.Parse)
- Never use regex patterns; rely on the actual libraries that will process the values
- This ensures compatibility: if Helm/Kubernetes accept it, our validator accepts it
- Example: `--chart` is required (Cobra), validated by `chartutil.ValidateMetadataName()` from Helm SDK

**Internal Packages**
- All business logic here, organized by concern
- Each package independently testable and mockable
- No global state or singletons

**Dependencies & Configuration**
- All functions accepting external dependencies take `*config.Config`
- This enables testing with mocks

**Error Handling**
- Use typed errors from `internal/errors/`
- Wrap with context at each layer
- Never swallow errors

**Logging**
- Structured key-value logging via `cfg.Logger`
- Log at operation start, external calls, errors
- Respect log levels

**Testing**
- Add tests alongside code changes
- Use table-driven tests
- Test happy paths and error cases
- Keep unit tests in package with `*_test.go` suffix

**Code Changes**
- Surgical changes consistent with existing patterns
- Prefer existing helpers
- Preserve Cobra flag patterns
- Add tests for manifest parsing, archive, registry changes

## cmd/ Testing

### Files & Coverage
- `cmd/pull_test.go` (25 tests) — flags, validation, execution paths
- `cmd/push_test.go` (17 tests) — flags, validation, execution paths  
- `cmd/root_test.go` (6 tests) — subcommand structure and routing
- `cmd/shared_test.go` — reusable test helpers

### Test Patterns
Use helpers from `cmd/shared_test.go`:
- `ExecuteCommand(cmd, args)` → run command, capture output
- `AssertFlag{Exists,Type,Default}()` → verify flag registration/properties
- `PatchCobraRunE(cmd, mockFunc)` → mock execution with auto state reset

Example:
```go
func TestPullCmd_ChartFlagRequired(t *testing.T) {
    output := ExecuteCommand(pullCmd, []string{})
    if output.Err == nil {
        t.Fatalf("expected error when --chart missing")
    }
}

restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
    return nil // mock backend
})
defer restore()
```

Tests verify: flag registration, types, defaults, required status, validation rules, and execution with valid inputs.

## Packages

- `internal/log/` — Structured logging (slog-based)
- `internal/config/` — Dependency injection Config struct with fluent builder
- `internal/validation/` — Reusable validators for flags and inputs
- `internal/errors/` — Typed error handling with context tracking
- `internal/chartmirror/` — Chart rendering and pull workflow
- `internal/chartimages/` — Image reference extraction from manifests
- `internal/mirror/` — OCI archive generation and push execution

## Commands

- `go test ./...` — Run tests
- `go build ./...` — Build
- `go run . --help` — Help
- `go run . pull --chart nginx --verbose` — Pull with verbose logging
- `go test ./... -run TestName` — Run specific test

## Notes

- `pull` loads charts in-process via Helm SDK, resolves versions from `index.yaml`, writes archives and `push_images.json`
- `push` reads `push_images.json`, pushes images to registry with bounded concurrency
- Temporary output in current directory unless `--output-dir` specified
