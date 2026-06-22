package validation

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid http URL", "http://example.com", false},
		{"valid https URL", "https://example.com/path", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"invalid URL", "ht!tp://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateChartName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid name", "my-chart", false},
		{"alphanumeric", "chart123", false},
		{"single char", "a", false},
		{"empty", "", true},
		{"uppercase", "MyChart", true},
		{"space", "my chart", true},
		{"starts with hyphen", "-mychart", true},
		{"ends with hyphen", "mychart-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChartName("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateChartName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"valid concurrency", 1, false},
		{"valid concurrency 4", 4, false},
		{"zero concurrency", 0, true},
		{"negative concurrency", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConcurrency("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConcurrency() error = %v, wantErr %v", err, tt.wantErr)
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
		name    string
		value   string
		wantErr bool
	}{
		{"valid registry", "localhost:5000", false},
		{"registry without port", "registry.example.com", false},
		{"registry with subdomain", "gcr.io", false},
		{"local registry", "localhost", false},
		{"empty", "", true},
		{"with protocol", "https://registry.example.com", true},
		{"with path", "registry.example.com/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageRegistry("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageRegistry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
