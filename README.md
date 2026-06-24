# helm-deep-pack

CLI tool to render Helm charts, extract referenced container images, stage them as OCI layout artifacts, and push them to a target registry.

## Build

```bash
go build ./...
```

## Pull chart images

```bash
go run . pull CHART [--repo REPO] [--version VERSION] [--output-dir DIR] [--concurrency N] [--allow-insecure-http]
```

Examples:

```bash
go run . pull prometheus-node-exporter --repo https://prometheus-community.github.io/helm-charts
go run . pull ./charts/my-local-chart
```

`--repo` uses HTTPS by default; use `--allow-insecure-http` only when a chart
repository is intentionally exposed over plain HTTP.

## Push staged images

```bash
go run . push REGISTRY [--input-dir DIR] [--concurrency N] [--all]
```

Example:

```bash
go run . push registry.internal:5000 --input-dir ./prometheus-node-exporter
```

If `--input-dir` is omitted, `push` looks for `push_images.json` first next to the
running executable, then in the current working directory.

### Interactive selection (default)

By default, `push` is interactive when run in a terminal. It probes the target
registry for each staged image and opens a checkbox list so you choose exactly
what to mirror — this avoids polluting the registry with images you didn't intend
to push.

Key bindings:

- `↑`/`↓` (or `k`/`j`) — move
- `space` — toggle the highlighted image
- `a` — toggle all
- `enter` — push the selected images
- `esc` / `Ctrl-C` — cancel without pushing

Each image is annotated with its status in the target registry:

- `[missing]` — not present in the registry yet
- `[exists]` — already present with a matching digest
- `[conflict]` — the same reference exists with a different digest; if selected, push updates that destination reference to the staged digest
- `[unknown]` — the registry check failed (a warning is printed); still selectable

If your selection includes any `[conflict]` images, pressing `enter` shows an
orange warning with the conflicting digest details (`current` vs `staged`),
potential risks (overwriting existing references, runtime drift, rollback
complexity), and asks for explicit `yes/no` confirmation before pushing.

Nothing is pre-selected. Confirming with no images checked exits cleanly without
pushing.

### Non-interactive push

Use `--all` to skip the prompt and push every staged image. This is required when
there is no terminal (for example in CI); without `--all` in a non-interactive
environment the command exits with an error.

```bash
go run . push registry.internal:5000 --input-dir ./prometheus-node-exporter --all
```

## OCI Helm registries

`pull` supports OCI-hosted charts in both forms:

```bash
go run . pull oci://registry.example.com/charts/mychart --version 1.2.3
go run . pull mychart --repo oci://registry.example.com/charts --version 1.2.3
```

For private registries, authenticate first with Helm:

```bash
helm registry login registry.example.com
```

The CLI reuses Helm/Docker registry credentials automatically.

For local OCI registries on `localhost`, plain HTTP is enabled automatically.

Example that works today:

```bash
go run . pull oci://registry-1.docker.io/bitnamicharts/nginx --version 25.0.10
```
