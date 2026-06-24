# helm-deep-pack

`helm-deep-pack` renders Helm charts, finds referenced container images, stages them as OCI artifacts, and pushes them to your target registry.

## Install (latest stable)

macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/XScythe/helm-pull-images-cli/main/deploy/install.sh | sh
```

Windows (PowerShell):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "iwr -useb https://raw.githubusercontent.com/XScythe/helm-pull-images-cli/main/deploy/install.ps1 | iex"
```

Installers detect OS/architecture, download the latest stable release from GitHub, and install `helm-deep-pack`:

- macOS/Linux default: `/usr/local/bin` (or `HELM_DEEP_PACK_INSTALL`)
- Windows default: `%LOCALAPPDATA%\Programs\helm-deep-pack\bin` (or `HELM_DEEP_PACK_INSTALL`)

Pin a specific release:

```bash
HELM_DEEP_PACK_VERSION=v1.2.3 curl -fsSL https://raw.githubusercontent.com/XScythe/helm-pull-images-cli/main/deploy/install.sh | sh
```

```powershell
$env:HELM_DEEP_PACK_VERSION="v1.2.3"; iwr -useb https://raw.githubusercontent.com/XScythe/helm-pull-images-cli/main/deploy/install.ps1 | iex
```

## Quick start

Pull images from a chart into an output directory:

```bash
helm-deep-pack pull prometheus-node-exporter \
  --repo https://prometheus-community.github.io/helm-charts \
  --output-dir ./prometheus-node-exporter
```

Push staged images to a registry:

```bash
helm-deep-pack push registry.internal:5000 --input-dir ./prometheus-node-exporter
```

## Pull command

```bash
helm-deep-pack pull CHART [--repo REPO] [--version VERSION] [--output-dir DIR] [--concurrency N] [--allow-insecure-http]
```

- `CHART` can be a chart name, local chart path, or `oci://...` reference.
- `--repo` is HTTPS by default; use `--allow-insecure-http` only for intentionally plain-HTTP chart repositories.

Examples:

```bash
helm-deep-pack pull ./charts/my-local-chart
helm-deep-pack pull oci://registry.example.com/charts/mychart --version 1.2.3
```

## Push command

```bash
helm-deep-pack push REGISTRY [--input-dir DIR] [--concurrency N] [--all]
```

When run in a terminal, `push` is interactive by default so you can choose which images to mirror.

Use `--all` for non-interactive environments (for example CI):

```bash
helm-deep-pack push registry.internal:5000 --input-dir ./prometheus-node-exporter --all
```

If `--input-dir` is omitted, `push` looks for `push_images.json` next to the running executable, then in the current working directory.

## Private OCI chart registries

For private OCI registries, authenticate first:

```bash
helm registry login registry.example.com
```

`helm-deep-pack` reuses existing Helm/Docker registry credentials.

## Help

```bash
helm-deep-pack --help
helm-deep-pack pull --help
helm-deep-pack push --help
```
