package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	code := m.Run()
	// Clean up any test artifacts
	CleanupChartDirs()
	os.Exit(code)
}

// TestPullCmd_FlagsRegistered verifies all expected flags are registered on pull command.
func TestPullCmd_FlagsRegistered(t *testing.T) {
	flags := []string{
		"repo",
		"version",
		"output-dir",
		"concurrency",
		"verbose",
	}
	for _, flag := range flags {
		AssertFlagExists(t, pullCmd, flag)
	}
}

// TestPullCmd_FlagsNotExposed verifies that pull command doesn't expose registry flag.
// (This is push-only or not applicable to pull operations.)
func TestPullCmd_FlagsNotExposed(t *testing.T) {
	flags := []string{
		"registry",
	}
	for _, flag := range flags {
		AssertFlagNotExists(t, pullCmd, flag)
	}
}

// TestPullCmd_FlagTypes verifies all flags have the correct types.
func TestPullCmd_FlagTypes(t *testing.T) {
	tests := map[string]string{
		"repo":        "string",
		"version":     "string",
		"output-dir":  "string",
		"concurrency": "int",
		"verbose":     "bool",
	}
	for flagName, expectedType := range tests {
		AssertFlagType(t, pullCmd, flagName, expectedType)
	}
}

// TestPullCmd_FlagDefaults verifies all optional flags have correct defaults.
func TestPullCmd_FlagDefaults(t *testing.T) {
	tests := map[string]string{
		"repo":        "",
		"version":     "",
		"output-dir":  "",
		"concurrency": "4",
		"verbose":     "false",
	}
	for flagName, expectedDefault := range tests {
		AssertFlagDefault(t, pullCmd, flagName, expectedDefault)
	}
}

// TestPullCmd_ChartFlagRequired verifies that chart positional argument is required.
func TestPullCmd_ChartFlagRequired(t *testing.T) {
	// Execute pull without chart argument, expect error
	output := ExecuteCommand(pullCmd, []string{})
	if output.Err == nil {
		t.Fatalf("expected error when chart argument is missing, got none")
	}
	// Error should mention missing argument
	errOutput := output.Stderr + output.Stdout
	if !strings.Contains(errOutput, "arg(s)") && !strings.Contains(errOutput, "received 0") {
		t.Fatalf("error message should mention missing args, got: %s", errOutput)
	}
}

// TestPullCmd_ValidateChartNameInvalid tests invalid chart names are rejected.
func TestPullCmd_ValidateChartNameInvalid(t *testing.T) {
	// Helm chart names must follow DNS-1123 subdomain rules
	invalidNames := []string{
		"Chart_Name", // underscore not allowed
		"CHART",      // uppercase allowed, but test invalid syntax
		"-chart",     // cannot start with dash
		"chart-",     // cannot end with dash
		"chart@",     // special chars not allowed
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			output := ExecuteCommand(pullCmd, []string{name})
			if output.Err == nil {
				t.Fatalf("expected error for invalid chart name %q, got none", name)
			}
		})
	}
}

// TestPullCmd_ValidateChartNameValid tests valid chart names are accepted.
func TestPullCmd_ValidateChartNameValid(t *testing.T) {
	validNames := []string{
		"nginx",
		"nginx-ingress",
		"prometheus-operator",
		"my-chart",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
				return nil // Mock success
			})
			defer restore()

			output := ExecuteCommand(pullCmd, []string{name})
			if output.Err != nil {
				t.Fatalf("expected valid chart name %q to pass validation, got: %v", name, output.Err)
			}
		})
	}
}

// TestPullCmd_ValidateRepoURLInvalid tests invalid repo URLs are rejected.
func TestPullCmd_ValidateRepoURLInvalid(t *testing.T) {
	invalidURLs := []string{
		"not-a-url",
		"ftp://example.com",   // only http/https allowed conceptually
		"example.com",         // missing scheme
		"ht!tp://example.com", // invalid scheme
	}

	for _, url := range invalidURLs {
		t.Run(url, func(t *testing.T) {
			output := ExecuteCommand(pullCmd, []string{"nginx", "--repo", url})
			if output.Err == nil {
				t.Fatalf("expected error for invalid repo URL %q, got none", url)
			}
		})
	}
}

// TestPullCmd_ValidateRepoURLValid tests valid repo URLs are accepted.
func TestPullCmd_ValidateRepoURLValid(t *testing.T) {
	validURLs := []string{
		"https://charts.example.com",
		"http://charts.example.com",
		"https://kubernetes.github.io/ingress-nginx",
	}

	for _, url := range validURLs {
		t.Run(url, func(t *testing.T) {
			restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
				return nil // Mock success
			})
			defer restore()

			output := ExecuteCommand(pullCmd, []string{"nginx", "--repo", url})
			if output.Err != nil {
				t.Fatalf("expected valid repo URL %q to pass validation, got: %v", url, output.Err)
			}
		})
	}
}

// TestPullCmd_ValidateConcurrencyInvalid tests invalid concurrency values are rejected.
func TestPullCmd_ValidateConcurrencyInvalid(t *testing.T) {
	tests := map[string]string{
		"zero":     "0",
		"negative": "-1",
		"toolarge": "10000", // exceeds cpu*4 limit
	}

	for testName, concurrency := range tests {
		t.Run(testName, func(t *testing.T) {
			output := ExecuteCommand(pullCmd, []string{"nginx", "--concurrency", concurrency})
			if output.Err == nil {
				t.Fatalf("expected error for concurrency=%s, got none", concurrency)
			}
		})
	}
}

// TestPullCmd_ValidateConcurrencyValid tests valid concurrency values are accepted.
func TestPullCmd_ValidateConcurrencyValid(t *testing.T) {
	validConcurrencies := []string{"1", "4", "8", "16"}

	for _, concurrency := range validConcurrencies {
		t.Run(concurrency, func(t *testing.T) {
			restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
				return nil // Mock success
			})
			defer restore()

			output := ExecuteCommand(pullCmd, []string{"nginx", "--concurrency", concurrency})
			if output.Err != nil {
				t.Fatalf("expected valid concurrency %s to pass validation, got: %v", concurrency, output.Err)
			}
		})
	}
}

// TestPullCmd_ExecuteMinimal tests pull command with minimal valid arguments (chart only).
func TestPullCmd_ExecuteMinimal(t *testing.T) {
	// Mock the pull.Run function to avoid real I/O
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		// Simulate successful command execution
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{"nginx"})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteWithRepo tests pull command with chart and repo.
func TestPullCmd_ExecuteWithRepo(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--repo", "https://charts.example.com",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteWithVersion tests pull command with chart and version.
func TestPullCmd_ExecuteWithVersion(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--version", "1.0.0",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteWithOutputDir tests pull command with output directory.
func TestPullCmd_ExecuteWithOutputDir(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--output-dir", "/tmp/output",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteWithConcurrency tests pull command with custom concurrency.
func TestPullCmd_ExecuteWithConcurrency(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--concurrency", "8",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteWithVerbose tests pull command with verbose logging enabled.
func TestPullCmd_ExecuteWithVerbose(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--verbose",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_ExecuteAllFlags tests pull command with all flags specified.
func TestPullCmd_ExecuteAllFlags(t *testing.T) {
	restore := PatchCobraRunE(pullCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--repo", "https://charts.bitnami.com/bitnami",
		"--version", "14.0.0",
		"--output-dir", "/tmp/nginx-mirror",
		"--concurrency", "8",
		"--verbose",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPullCmd_HelpFlag tests that pull command supports --help flag.
func TestPullCmd_HelpFlag(t *testing.T) {
	output := ExecuteCommand(pullCmd, []string{"--help"})
	if output.Err != nil {
		t.Fatalf("--help should not produce error: %v", output.Err)
	}
	// Help output should contain command description
	helpOutput := output.Stdout + output.Stderr
	if !strings.Contains(helpOutput, "Render") || !strings.Contains(helpOutput, "chart") {
		t.Fatalf("help output missing expected content: %s", helpOutput)
	}
}

// TestPullCmd_UnknownFlag tests that unknown flags are rejected.
func TestPullCmd_UnknownFlag(t *testing.T) {
	output := ExecuteCommand(pullCmd, []string{
		"nginx",
		"--unknown-flag", "value",
	})
	if output.Err == nil {
		t.Fatalf("expected error for unknown flag, got none")
	}
}

// Stub to allow command testing without external dependencies.
var _ context.Context
