package pull

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

const ociScheme = "oci://"

func isOCIRef(value string) bool {
	return strings.HasPrefix(strings.ToLower(value), ociScheme)
}

func trimOCIScheme(value string) string {
	return value[len(ociScheme):]
}

func ociChartRef(opts Options) (string, error) {
	switch {
	case isOCIRef(opts.Chart):
		ref := trimOCIScheme(opts.Chart)
		if opts.Version == "" && !refHasTagOrDigest(ref) {
			return "", fmt.Errorf("resolve OCI chart reference: version is required for OCI charts (use --version or include :tag/@digest)")
		}
		if opts.Version != "" && !refHasTagOrDigest(ref) {
			ref += ":" + opts.Version
		}
		return ref, nil
	case isOCIRef(opts.Repo):
		chart := strings.TrimSpace(opts.Chart)
		if chart == "" {
			return "", fmt.Errorf("resolve OCI chart reference: chart name is required with --repo")
		}
		ref := strings.TrimSuffix(trimOCIScheme(opts.Repo), "/") + "/" + chart
		if opts.Version == "" && !refHasTagOrDigest(ref) {
			return "", fmt.Errorf("resolve OCI chart reference: version is required for OCI charts (use --version or include :tag/@digest)")
		}
		if opts.Version != "" && !refHasTagOrDigest(ref) {
			ref += ":" + opts.Version
		}
		return ref, nil
	default:
		return "", fmt.Errorf("resolve OCI chart reference: no OCI chart reference provided")
	}
}

func refHasTagOrDigest(ref string) bool {
	last := ref
	if idx := strings.LastIndex(last, "/"); idx >= 0 && idx < len(last)-1 {
		last = last[idx+1:]
	}
	return strings.Contains(last, "@") || strings.Contains(last, ":")
}

func isLocalRegistryHost(ref string) bool {
	host := ref
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func loadOCIChart(_ context.Context, opts Options) (loadedChart, error) {
	ref, err := ociChartRef(opts)
	if err != nil {
		return loadedChart{}, err
	}

	clientOpts := []registry.ClientOption{
		registry.ClientOptEnableCache(true),
	}
	if isLocalRegistryHost(ref) {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}

	client, err := registry.NewClient(clientOpts...)
	if err != nil {
		return loadedChart{}, fmt.Errorf("create OCI registry client: %w", err)
	}

	result, err := client.Pull(ref, registry.PullOptWithChart(true))
	if err != nil {
		return loadedChart{}, fmt.Errorf("pull OCI chart %q: %w", ociScheme+ref, err)
	}
	if len(result.Chart.Data) > maxChartArchiveBytes {
		return loadedChart{}, fmt.Errorf("chart archive exceeds size limit (%d bytes)", maxChartArchiveBytes)
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(result.Chart.Data))
	if err != nil {
		return loadedChart{}, fmt.Errorf("load OCI chart archive: %w", err)
	}

	return loadedChart{
		Chart:       chrt,
		ArchiveData: append([]byte(nil), result.Chart.Data...),
		ArchiveName: defaultChartArchiveName(chrt),
		Info: ChartInfo{
			Name:    chrt.Metadata.Name,
			Version: chrt.Metadata.Version,
			Source:  ociScheme + ref,
		},
	}, nil
}
