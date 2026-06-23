package pull

import (
	"context"
	"strings"
	"testing"
)

func TestIsOCIRef(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "oci://localhost:5000/charts/demo", want: true},
		{value: "OCI://localhost:5000/charts/demo", want: true},
		{value: "https://charts.example.com/demo", want: false},
		{value: "demo", want: false},
	}

	for _, tt := range tests {
		if got := isOCIRef(tt.value); got != tt.want {
			t.Fatalf("isOCIRef(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestOCIChartRef(t *testing.T) {
	tests := []struct {
		name            string
		opts            Options
		want            string
		wantErrContains string
	}{
		{
			name: "full oci chart ref with version appends tag",
			opts: Options{
				Chart:   "oci://registry.example.com/charts/demo",
				Version: "1.2.3",
			},
			want: "registry.example.com/charts/demo:1.2.3",
		},
		{
			name: "full oci chart ref keeps existing tag",
			opts: Options{
				Chart:   "oci://registry.example.com/charts/demo:9.9.9",
				Version: "1.2.3",
			},
			want: "registry.example.com/charts/demo:9.9.9",
		},
		{
			name: "full oci chart ref keeps existing digest",
			opts: Options{
				Chart:   "oci://registry.example.com/charts/demo@sha256:abc",
				Version: "1.2.3",
			},
			want: "registry.example.com/charts/demo@sha256:abc",
		},
		{
			name: "repo split oci ref",
			opts: Options{
				Chart:   "demo",
				Repo:    "oci://localhost:5000/charts",
				Version: "2.0.0",
			},
			want: "localhost:5000/charts/demo:2.0.0",
		},
		{
			name: "full oci chart ref without version errors",
			opts: Options{
				Chart: "oci://registry.example.com/charts/demo",
			},
			wantErrContains: "version is required for OCI charts",
		},
		{
			name: "repo split oci ref without version errors",
			opts: Options{
				Chart: "demo",
				Repo:  "oci://localhost:5000/charts",
			},
			wantErrContains: "version is required for OCI charts",
		},
		{
			name: "non oci ref errors",
			opts: Options{
				Chart: "demo",
				Repo:  "https://charts.example.com",
			},
			wantErrContains: "no OCI chart reference provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ociChartRef(tt.opts)
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("ociChartRef() error = %v, want containing %q", err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("ociChartRef() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ociChartRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefHasTagOrDigest(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "localhost:5000/charts/demo", want: false},
		{ref: "localhost:5000/charts/demo:1.2.3", want: true},
		{ref: "localhost:5000/charts/demo@sha256:abc", want: true},
	}

	for _, tt := range tests {
		if got := refHasTagOrDigest(tt.ref); got != tt.want {
			t.Fatalf("refHasTagOrDigest(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestIsLocalRegistryHost(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "localhost:5000/charts/demo:1.2.3", want: true},
		{ref: "127.0.0.1:5000/charts/demo:1.2.3", want: true},
		{ref: "[::1]:5000/charts/demo:1.2.3", want: true},
		{ref: "registry.example.com/charts/demo:1.2.3", want: false},
	}

	for _, tt := range tests {
		if got := isLocalRegistryHost(tt.ref); got != tt.want {
			t.Fatalf("isLocalRegistryHost(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestLoadChartUsesOCIChartSource(t *testing.T) {
	h := newRunnerTestHarness(t)

	localCalled := false
	repoCalled := false
	ociCalls := 0
	h.runner.localChartSource = func(context.Context, Options) (loadedChart, error) {
		localCalled = true
		return loadedChart{}, nil
	}
	h.runner.helmChartSource = func(context.Context, Options) (loadedChart, error) {
		repoCalled = true
		return loadedChart{}, nil
	}
	h.runner.ociChartSource = func(_ context.Context, opts Options) (loadedChart, error) {
		ociCalls++
		return testLoadedChart("oci", "1.0.0", opts.Chart), nil
	}

	opts := Options{Chart: "oci://localhost:5000/charts/demo", Version: "1.0.0"}
	first, err := h.runner.loadChart(context.Background(), opts)
	if err != nil {
		t.Fatalf("loadChart() first call error = %v", err)
	}
	second, err := h.runner.loadChart(context.Background(), opts)
	if err != nil {
		t.Fatalf("loadChart() second call error = %v", err)
	}

	if localCalled {
		t.Fatal("local chart source should not be called for OCI references")
	}
	if repoCalled {
		t.Fatal("helm repo source should not be called for OCI references")
	}
	if ociCalls != 1 {
		t.Fatalf("oci chart source call count = %d, want 1 (cached after first call)", ociCalls)
	}
	if first.Info.Name != "oci" || second.Info.Name != "oci" {
		t.Fatalf("unexpected OCI chart info: first=%+v second=%+v", first.Info, second.Info)
	}
}
