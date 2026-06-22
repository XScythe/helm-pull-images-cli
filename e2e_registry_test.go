package main_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v3"
	"helm-pull-images-cli/internal/mirror"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	e2eRegistryHost           = "localhost:5000"
	e2eSmallChartsRepoURL     = "https://prometheus-community.github.io/helm-charts"
	e2eLargeChartsRepoURL     = "https://prometheus-community.github.io/helm-charts"
	e2eSmallChartName         = "prometheus-node-exporter"
	e2eLargeRemoteChartName   = "prometheus"
	e2eSmallChartMinImages    = 1
	e2eLargeRemoteMinImageSet = 5
)

func TestRegistryE2E(t *testing.T) {
	if os.Getenv("E2E_REGISTRY_TEST") == "" {
		t.Skip("set E2E_REGISTRY_TEST=1 to run the registry e2e test")
	}

	if testing.Short() {
		t.Skip("skipping registry e2e test in short mode")
	}

	t.Log("ensuring registry")
	containerID, err := ensureRegistry(t)
	if err != nil {
		t.Fatalf("ensure registry: %v", err)
	}
	if containerID != "" {
		t.Cleanup(func() {
			stopRegistryContainer(t, containerID)
		})
	}

	cliBinary := buildCLIBinary(t)
	localChartRoot, localChartName, localChartVersion := preparePopularLocalChart(t, e2eSmallChartName, e2eSmallChartsRepoURL)
	largeChartVersion := resolveStableChartVersion(t, e2eLargeChartsRepoURL, e2eLargeRemoteChartName)

	testCases := []struct {
		name       string
		chart      string
		repo       string
		version    string
		workingDir string
		minImages  int
	}{
		{
			name:       "small local popular chart",
			chart:      localChartName,
			version:    localChartVersion,
			workingDir: localChartRoot,
			minImages:  e2eSmallChartMinImages,
		},
		{
			name:      "large remote popular chart",
			chart:     e2eLargeRemoteChartName,
			repo:      e2eLargeChartsRepoURL,
			version:   largeChartVersion,
			minImages: e2eLargeRemoteMinImageSet,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runRegistryE2ECase(t, cliBinary, tc.chart, tc.repo, tc.version, tc.workingDir, tc.minImages)
		})
	}
}

func runRegistryE2ECase(t *testing.T, cliBinary, chart, repoURL, chartVersion, workingDir string, minImages int) {
	t.Helper()

	outputDir := t.TempDir()
	t.Log("rendering chart and archiving images")
	pullArgs := []string{"pull", "--chart", chart, "--output-dir", outputDir}
	if repoURL != "" {
		pullArgs = append(pullArgs, "--repo", repoURL)
	}
	if chartVersion != "" {
		pullArgs = append(pullArgs, "--version", chartVersion)
	}
	pullCmd := exec.Command(cliBinary, pullArgs...)
	if workingDir != "" {
		pullCmd.Dir = workingDir
	}
	pullOutput, err := pullCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pull command failed: %v\nargs: %v\n%s", err, pullArgs, string(pullOutput))
	}

	t.Log("checking artifacts")
	manifest := checkE2EArtifacts(t, outputDir, minImages)

	pushBinary := filepath.Join(outputDir, mirror.PushBinaryName())
	t.Log("pushing mirrored images")
	cmd := exec.Command(pushBinary, "push", "--registry", e2eRegistryHost)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("push helper failed: %v\n%s", err, string(output))
	}

	t.Log("verifying registry images")
	verifyRegistryImages(t, e2eRegistryHost, manifest)
}

func preparePopularLocalChart(t *testing.T, chartName, repoURL string) (string, string, string) {
	t.Helper()

	chartVersion := resolveStableChartVersion(t, repoURL, chartName)
	chartURL, err := repo.FindChartInRepoURL(repoURL, chartName, chartVersion, "", "", "", getter.All(cli.New()))
	if err != nil {
		t.Fatalf("find chart URL for %s/%s:%s: %v", repoURL, chartName, chartVersion, err)
	}

	archive := downloadChartArchive(t, chartURL)
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("load chart archive %s: %v", chartURL, err)
	}

	rootDir := t.TempDir()
	if err := chartutil.SaveDir(loadedChart, rootDir); err != nil {
		t.Fatalf("save chart %q locally: %v", loadedChart.Metadata.Name, err)
	}

	return rootDir, loadedChart.Metadata.Name, chartVersion
}

func resolveStableChartVersion(t *testing.T, repoURL, chartName string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, strings.TrimRight(repoURL, "/")+"/index.yaml", nil)
	if err != nil {
		t.Fatalf("prepare chart index request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch chart index: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("fetch chart index: %s: %s", resp.Status, string(body))
	}

	var index struct {
		Entries map[string][]struct {
			Version string `yaml:"version"`
		} `yaml:"entries"`
	}
	if err := yaml.NewDecoder(resp.Body).Decode(&index); err != nil {
		t.Fatalf("decode chart index: %v", err)
	}

	results := index.Entries[chartName]
	for _, result := range results {
		if result.Version != "" && !strings.Contains(result.Version, "-") {
			return result.Version
		}
	}

	t.Fatalf("no stable version found for %s/%s", repoURL, chartName)
	return ""
}

func downloadChartArchive(t *testing.T, chartURL string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chartURL, nil)
	if err != nil {
		t.Fatalf("prepare chart archive request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch chart archive: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("fetch chart archive: %s: %s", resp.Status, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read chart archive response: %v", err)
	}
	return data
}

func ensureRegistry(t *testing.T) (string, error) {
	t.Helper()

	if registryIsReady() {
		return "", nil
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("no registry on :5000 and docker is unavailable")
	}

	containerName := fmt.Sprintf("helm-pull-images-e2e-%d", time.Now().UnixNano())
	cmd := exec.Command("docker", "run", "-d", "--rm", "--name", containerName, "-p", "127.0.0.1:5000:5000", "registry:3")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run registry: %w: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	waitForRegistry(t)
	return containerID, nil
}

func stopRegistryContainer(t *testing.T, containerID string) {
	t.Helper()

	cmd := exec.Command("docker", "stop", "--timeout", "5", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(output)
		if strings.Contains(msg, "No such container") {
			return
		}
		t.Fatalf("docker stop %s: %v: %s", containerID, err, msg)
	}
}

func registryIsReady() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + e2eRegistryHost + "/v2/")
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	return resp.StatusCode == http.StatusOK
}

func waitForRegistry(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if registryIsReady() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("registry at %s did not become ready", e2eRegistryHost)
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "helm-pull-images-cli")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build CLI binary: %v: %s", err, string(output))
	}
	return binary
}

func checkE2EArtifacts(t *testing.T, outputDir string, minImages int) *mirror.PushManifest {
	t.Helper()

	if _, err := os.Stat(filepath.Join(outputDir, mirror.PushBinaryName())); err != nil {
		t.Fatalf("missing push helper binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, mirror.PushManifestFileName())); err != nil {
		t.Fatalf("missing push manifest: %v", err)
	}
	if info, err := os.Stat(filepath.Join(outputDir, mirror.OCILayoutDirName())); err != nil || !info.IsDir() {
		t.Fatalf("missing OCI layout directory: %v", err)
	}

	manifest, err := mirror.ReadPushManifest(outputDir)
	if err != nil {
		t.Fatalf("ReadPushManifest() error = %v", err)
	}
	if len(manifest.Images) < minImages {
		t.Fatalf("push manifest images len = %d, want at least %d", len(manifest.Images), minImages)
	}
	for i, spec := range manifest.Images {
		if spec.Image == "" {
			t.Fatalf("push manifest image[%d] has empty source image", i)
		}
		if spec.Target == "" {
			t.Fatalf("push manifest image[%d] has empty target", i)
		}
		if spec.OCIDigest == "" {
			t.Fatalf("push manifest image[%d] has empty oci digest", i)
		}
	}
	return manifest
}

func verifyRegistryImages(t *testing.T, registryHost string, manifest *mirror.PushManifest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	checked := 0
	for _, spec := range manifest.Images {
		destRef, err := name.ParseReference(fmt.Sprintf("%s/%s", registryHost, spec.Target), name.Insecure)
		if err != nil {
			t.Fatalf("parse destination reference for %q: %v", spec.Target, err)
		}
		destImage, err := remote.Image(destRef, remote.WithContext(ctx))
		if err != nil {
			t.Fatalf("remote.Image(%s): %v", destRef.String(), err)
		}
		if _, err := destImage.ConfigFile(); err != nil {
			t.Fatalf("destination image config %s: %v", destRef.String(), err)
		}

		sourceRef, err := name.ParseReference(spec.Image)
		if err != nil {
			t.Fatalf("parse source reference for %q: %v", spec.Image, err)
		}
		sourceImage, err := remote.Image(sourceRef, remote.WithContext(ctx))
		if err != nil {
			t.Fatalf("remote.Image(%s): %v", sourceRef.String(), err)
		}

		destDigest, err := destImage.Digest()
		if err != nil {
			t.Fatalf("destination digest %s: %v", destRef.String(), err)
		}
		sourceDigest, err := sourceImage.Digest()
		if err != nil {
			t.Fatalf("source digest %s: %v", sourceRef.String(), err)
		}
		if destDigest != sourceDigest {
			t.Fatalf("uploaded digest for %s = %s, want %s", spec.Image, destDigest, sourceDigest)
		}
		if spec.OCIDigest != sourceDigest.String() {
			t.Fatalf("manifest oci digest for %s = %s, want %s", spec.Image, spec.OCIDigest, sourceDigest.String())
		}
		checked++
	}

	if checked != len(manifest.Images) {
		t.Fatalf("verified %d images, want %d", checked, len(manifest.Images))
	}
}
