package main_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"helm-pull-images-cli/internal/mirror"
)

const (
	e2eRegistryHost = "localhost:5000"
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

	t.Log("building source image")
	sourceImage := buildSourceImage(t)
	t.Logf("built source image %s", sourceImage)
	chartDir := writeLocalChart(t, sourceImage)
	outputDir := t.TempDir()

	cliBinary := buildCLIBinary(t)

	t.Log("rendering chart and archiving images")
	pullCmd := exec.Command(cliBinary, "pull",
		"--chart", chartDir,
		"--output-dir", outputDir,
		"--release-name", "mirror",
		"--namespace", "default",
	)
	pullOutput, err := pullCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pull command failed: %v\n%s", err, string(pullOutput))
	}

	t.Log("checking artifacts")
	checkE2EArtifacts(t, outputDir, sourceImage)

	pushBinary := filepath.Join(outputDir, mirror.PushBinaryName())
	t.Log("pushing mirrored images")
	cmd := exec.Command(pushBinary, "push", "--registry", e2eRegistryHost)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("push helper failed: %v\n%s", err, string(output))
	}

	t.Log("verifying registry image")
	verifyRegistryImage(t, e2eRegistryHost, sourceImage)
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
	defer resp.Body.Close()
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

func writeLocalChart(t *testing.T, image string) string {
	t.Helper()

	chartDir := t.TempDir()
	mustWrite(t, filepath.Join(chartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: e2e-chart
version: 0.1.0
`))
	mustWrite(t, filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e
spec:
  selector:
    matchLabels:
      app: e2e
  template:
    metadata:
      labels:
        app: e2e
    spec:
      containers:
        - name: app
          image: %s
`, image)))
	return chartDir
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

func buildSourceImage(t *testing.T) string {
	t.Helper()

	image := fmt.Sprintf("%s/e2e/source:%d", e2eRegistryHost, time.Now().UnixNano())
	buildDir := t.TempDir()
	mustWrite(t, filepath.Join(buildDir, "Dockerfile"), []byte(`FROM scratch
COPY payload.txt /payload.txt
`))
	mustWrite(t, filepath.Join(buildDir, "payload.txt"), []byte("e2e"))

	cmd := exec.Command("docker", "build", "-t", image, buildDir)
	t.Logf("docker build %s", image)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker build %s: %v: %s", image, err, string(output))
	}

	cmd = exec.Command("docker", "push", image)
	t.Logf("docker push %s", image)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker push %s: %v: %s", image, err, string(output))
	}

	return image
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func checkE2EArtifacts(t *testing.T, outputDir, sourceImage string) {
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
	if len(manifest.Images) != 1 {
		t.Fatalf("push manifest images len = %d, want 1", len(manifest.Images))
	}
	if manifest.Images[0].Image != sourceImage {
		t.Fatalf("push manifest image = %q, want %q", manifest.Images[0].Image, sourceImage)
	}
	if manifest.Images[0].OCIDigest == "" {
		t.Fatal("push manifest missing oci digest")
	}
}

func verifyRegistryImage(t *testing.T, registryHost, sourceImage string) {
	t.Helper()

	specs, err := mirror.BuildSpecs([]string{sourceImage})
	if err != nil {
		t.Fatalf("BuildSpecs() error = %v", err)
	}
	spec := specs[0]
	destRef, err := name.NewTag(fmt.Sprintf("%s/%s", registryHost, spec.Target), name.Insecure)
	if err != nil {
		t.Fatalf("parse destination reference: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got, err := remote.Image(destRef, remote.WithContext(ctx))
	if err != nil {
		t.Fatalf("remote.Image(%s): %v", destRef.String(), err)
	}

	sourceRef, err := name.NewTag(sourceImage)
	if err != nil {
		t.Fatalf("parse source reference: %v", err)
	}
	source, err := remote.Image(sourceRef, remote.WithContext(ctx))
	if err != nil {
		t.Fatalf("remote.Image(%s): %v", sourceRef.String(), err)
	}

	gotDigest, err := got.Digest()
	if err != nil {
		t.Fatalf("destination digest: %v", err)
	}
	sourceDigest, err := source.Digest()
	if err != nil {
		t.Fatalf("source digest: %v", err)
	}
	if gotDigest != sourceDigest {
		t.Fatalf("uploaded digest = %s, want %s", gotDigest, sourceDigest)
	}
}
