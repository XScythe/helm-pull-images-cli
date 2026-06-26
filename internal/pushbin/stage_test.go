package pushbin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStageFallsBackToSelfCopyWhenEmbeddedPayloadIsPlaceholder(t *testing.T) {
	originalRead := readEmbeddedBinary
	originalCopy := copySelfExecutable
	defer func() {
		readEmbeddedBinary = originalRead
		copySelfExecutable = originalCopy
	}()

	readEmbeddedBinary = func() ([]byte, error) {
		return []byte("PLACEHOLDER\n"), nil
	}

	var copiedOutputDir string
	copySelfExecutable = func(outputDir string) (string, error) {
		copiedOutputDir = outputDir
		return filepath.Join(outputDir, "fallback"), nil
	}

	outDir := t.TempDir()
	got, err := Stage(outDir)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if copiedOutputDir != outDir {
		t.Fatalf("copySelfExecutable outputDir = %q, want %q", copiedOutputDir, outDir)
	}
	if got != filepath.Join(outDir, "fallback") {
		t.Fatalf("Stage() = %q", got)
	}
}

func TestStageWritesEmbeddedPayload(t *testing.T) {
	originalRead := readEmbeddedBinary
	originalCopy := copySelfExecutable
	defer func() {
		readEmbeddedBinary = originalRead
		copySelfExecutable = originalCopy
	}()

	readEmbeddedBinary = func() ([]byte, error) {
		return []byte("ELF-BINARY-DATA"), nil
	}
	copySelfExecutable = func(outputDir string) (string, error) {
		return filepath.Join(outputDir, "fallback"), nil
	}

	outDir := t.TempDir()
	path, err := Stage(outDir)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if path != filepath.Join(outDir, "push_images") && path != filepath.Join(outDir, "push_images.exe") {
		t.Fatalf("Stage() path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "ELF-BINARY-DATA" {
		t.Fatalf("staged payload = %q", string(data))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("mode = %#o, want %#o", info.Mode().Perm(), 0o755)
		}
	}
}

func TestHasEmbeddedPayload(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "empty", data: nil, want: false},
		{name: "placeholder", data: []byte("PLACEHOLDER"), want: false},
		{name: "placeholder with newline", data: []byte("PLACEHOLDER\n"), want: false},
		{name: "real payload", data: []byte("binary"), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasEmbeddedPayload(tc.data); got != tc.want {
				t.Fatalf("hasEmbeddedPayload(%q) = %v, want %v", string(tc.data), got, tc.want)
			}
		})
	}
}
