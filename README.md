# helm-deep-pack

CLI tool to render Helm charts, extract referenced container images, stage them as OCI layout artifacts, and push them to a target registry.

## Build

```bash
go build ./...
```

## Pull chart images

```bash
go run . pull CHART [--repo REPO] [--version VERSION] [--output-dir DIR] [--concurrency N]
```

Examples:

```bash
go run . pull prometheus-node-exporter --repo https://prometheus-community.github.io/helm-charts
go run . pull ./charts/my-local-chart
```

## Push staged images

```bash
go run . push REGISTRY [--input-dir DIR] [--concurrency N]
```

Example:

```bash
go run . push registry.internal:5000 --input-dir ./prometheus-node-exporter
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
