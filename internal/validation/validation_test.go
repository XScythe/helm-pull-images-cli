package validation

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		allowInsecure   bool
		wantErr         bool
		wantErrContains string
	}{
		{"valid http URL with opt-in", "http://example.com", true, false, ""},
		{"plain http URL rejected by default", "http://example.com", false, true, "--allow-insecure-http"},
		{"valid https URL", "https://example.com/path", false, false, ""},
		{"empty", "", false, true, "valid URL"},
		{"no scheme", "example.com", false, true, "scheme"},
		{"invalid URL", "ht!tp://example.com", false, true, "valid URL"},
		{"unsupported scheme", "ftp://example.com", false, true, "https scheme"},
		{"oci scheme", "oci://registry.example.com/charts", false, true, "https scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL("test", tt.value, tt.allowInsecure)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("ValidateURL() error = %v, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

func TestValidateOCIRef(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		wantErr         bool
		wantErrContains string
	}{
		{"valid chart ref", "oci://registry.example.com/charts/my-chart", false, ""},
		{"valid localhost ref", "oci://localhost:5000/charts/my-chart:1.2.3", false, ""},
		{"valid uppercase scheme", "OCI://localhost:5000/charts/my-chart", false, ""},
		{"missing scheme", "registry.example.com/charts/my-chart", true, "must start with oci://"},
		{"missing host", "oci:///charts/my-chart", true, "host and chart path"},
		{"missing path", "oci://registry.example.com", true, "host and chart path"},
		{"empty path segment", "oci://registry.example.com/charts//my-chart", true, "non-empty chart path"},
		{"whitespace", "oci://registry.example.com/charts/my chart", true, "whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOCIRef("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOCIRef() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("ValidateOCIRef() error = %v, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

func TestValidateChartName(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		wantErr         bool
		wantErrContains string
	}{
		{"valid name", "my-chart", false, ""},
		{"alphanumeric", "chart123", false, ""},
		{"single char", "a", false, ""},
		{"empty", "", true, "invalid metadata name"},
		{"uppercase", "MyChart", true, "invalid metadata name"},
		{"space", "my chart", true, "invalid metadata name"},
		{"starts with hyphen", "-mychart", true, "invalid metadata name"},
		{"ends with hyphen", "mychart-", true, "invalid metadata name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChartName("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateChartName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("ValidateChartName() error = %v, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name            string
		value           int
		wantErr         bool
		wantErrContains string
	}{
		{"valid concurrency", 1, false, ""},
		{"valid concurrency 4", 4, false, ""},
		{"zero concurrency", 0, true, "must be at least 1"},
		{"negative concurrency", -1, true, "must be at least 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConcurrency("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConcurrency() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("ValidateConcurrency() error = %v, want to contain %q", err, tt.wantErrContains)
			}
		})
	}

	// Test that max is derived from system CPUs
	maxValue := maxConcurrency()
	if err := ValidateConcurrency("test", maxValue); err != nil {
		t.Errorf("ValidateConcurrency(maxValue=%d) should not error: %v", maxValue, err)
	}

	if err := ValidateConcurrency("test", maxValue+1); err == nil {
		t.Errorf("ValidateConcurrency(maxValue+1=%d) should error", maxValue+1)
	} else if !strings.Contains(err.Error(), "must not exceed") {
		t.Errorf("ValidateConcurrency(maxValue+1=%d) error = %v, want to contain %q", maxValue+1, err, "must not exceed")
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid version", "1.2.3", false},
		{"empty version", "", false},
		{"version with dash", "1.2.3-alpha", false},
		{"version with v prefix", "v1.2.3", false}, // Allows v prefix (validation deferred to runtime)
		{"complex version", "1.2.3-rc1+build.123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateImageRegistry(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		wantErr         bool
		wantErrContains string
	}{
		{"valid registry", "localhost:5000", false, ""},
		{"registry without port", "registry.example.com", false, ""},
		{"registry with subdomain", "gcr.io", false, ""},
		{"local registry", "localhost", false, ""},
		{"empty", "", true, "must not be empty"},
		{"with protocol", "https://registry.example.com", true, "should not include protocol scheme"},
		{"with path", "registry.example.com/path", true, "should not include path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageRegistry("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageRegistry() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("ValidateImageRegistry() error = %v, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}
