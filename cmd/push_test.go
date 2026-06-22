package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

// TestPushCmd_FlagsRegistered verifies all expected flags are registered on push command.
func TestPushCmd_FlagsRegistered(t *testing.T) {
	flags := []string{
		"registry",
		"input-dir",
		"concurrency",
		"verbose",
	}
	for _, flag := range flags {
		AssertFlagExists(t, pushCmd, flag)
	}
}

// TestPushCmd_FlagsNotExposed verifies that push command doesn't expose pull-specific flags.
func TestPushCmd_FlagsNotExposed(t *testing.T) {
	flags := []string{
		"chart",
		"repo",
		"version",
		"output-dir",
	}
	for _, flag := range flags {
		AssertFlagNotExists(t, pushCmd, flag)
	}
}

// TestPushCmd_FlagTypes verifies all flags have the correct types.
func TestPushCmd_FlagTypes(t *testing.T) {
	tests := map[string]string{
		"registry":    "string",
		"input-dir":   "string",
		"concurrency": "int",
		"verbose":     "bool",
	}
	for flagName, expectedType := range tests {
		AssertFlagType(t, pushCmd, flagName, expectedType)
	}
}

// TestPushCmd_FlagDefaults verifies all optional flags have correct defaults.
func TestPushCmd_FlagDefaults(t *testing.T) {
	tests := map[string]string{
		"input-dir":   "",
		"concurrency": "4",
		"verbose":     "false",
	}
	for flagName, expectedDefault := range tests {
		AssertFlagDefault(t, pushCmd, flagName, expectedDefault)
	}
}

// TestPushCmd_RegistryFlagRequired verifies that --registry is required.
func TestPushCmd_RegistryFlagRequired(t *testing.T) {
	// Execute push without --registry flag, expect error
	output := ExecuteCommand(pushCmd, []string{})
	if output.Err == nil {
		t.Fatalf("expected error when --registry is missing, got none")
	}
	// Error should mention the missing flag
	errOutput := output.Stderr + output.Stdout
	if !containsSubstring(errOutput, "registry") && !containsSubstring(errOutput, "required") {
		t.Fatalf("error message should mention 'registry' or 'required', got: %s", errOutput)
	}
}

// TestPushCmd_ValidateRegistryInvalid tests invalid registry values are rejected.
func TestPushCmd_ValidateRegistryInvalid(t *testing.T) {
	invalidRegistries := []string{
		"https://example.com",     // cannot include scheme
		"http://registry.example.com", // cannot include http/https
		"example.com/namespace",   // cannot include path
		"registry.example.com/path", // cannot include path
	}

	for _, registry := range invalidRegistries {
		t.Run(registry, func(t *testing.T) {
			output := ExecuteCommand(pushCmd, []string{"--registry", registry})
			if output.Err == nil {
				t.Fatalf("expected error for invalid registry %q, got none", registry)
			}
		})
	}
}

// TestPushCmd_ValidateRegistryValid tests valid registry values are accepted.
func TestPushCmd_ValidateRegistryValid(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil // Mock success
	})
	defer restore()

	validRegistries := []string{
		"docker.io",
		"registry.example.com",
		"registry.example.com:5000",
		"localhost:5000",
		"quay.io",
	}

	for _, registry := range validRegistries {
		t.Run(registry, func(t *testing.T) {
			output := ExecuteCommand(pushCmd, []string{"--registry", registry})
			if output.Err != nil {
				t.Fatalf("expected valid registry %q to pass validation, got: %v", registry, output.Err)
			}
		})
	}
}

// TestPushCmd_ValidateConcurrencyInvalid tests invalid concurrency values are rejected.
func TestPushCmd_ValidateConcurrencyInvalid(t *testing.T) {
	tests := map[string]string{
		"zero":     "0",
		"negative": "-1",
		"toolarge": "10000", // exceeds cpu*4 limit
	}

	for testName, concurrency := range tests {
		t.Run(testName, func(t *testing.T) {
			output := ExecuteCommand(pushCmd, []string{"--registry", "docker.io", "--concurrency", concurrency})
			if output.Err == nil {
				t.Fatalf("expected error for concurrency=%s, got none", concurrency)
			}
		})
	}
}

// TestPushCmd_ValidateConcurrencyValid tests valid concurrency values are accepted.
func TestPushCmd_ValidateConcurrencyValid(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil // Mock success
	})
	defer restore()

	validConcurrencies := []string{"1", "4", "8", "16"}

	for _, concurrency := range validConcurrencies {
		t.Run(concurrency, func(t *testing.T) {
			output := ExecuteCommand(pushCmd, []string{"--registry", "docker.io", "--concurrency", concurrency})
			if output.Err != nil {
				t.Fatalf("expected valid concurrency %s to pass validation, got: %v", concurrency, output.Err)
			}
		})
	}
}

// TestPushCmd_ExecuteMinimal tests push command with minimal valid arguments (registry only).
func TestPushCmd_ExecuteMinimal(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pushCmd, []string{"--registry", "docker.io"})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPushCmd_ExecuteWithInputDir tests push command with input directory.
func TestPushCmd_ExecuteWithInputDir(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pushCmd, []string{
		"--registry", "docker.io",
		"--input-dir", "/tmp/images",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPushCmd_ExecuteWithConcurrency tests push command with custom concurrency.
func TestPushCmd_ExecuteWithConcurrency(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pushCmd, []string{
		"--registry", "docker.io",
		"--concurrency", "8",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPushCmd_ExecuteWithVerbose tests push command with verbose logging enabled.
func TestPushCmd_ExecuteWithVerbose(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pushCmd, []string{
		"--registry", "docker.io",
		"--verbose",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPushCmd_ExecuteAllFlags tests push command with all flags specified.
func TestPushCmd_ExecuteAllFlags(t *testing.T) {
	restore := PatchCobraRunE(pushCmd, func(cmd *cobra.Command, args []string) error {
		return nil
	})
	defer restore()

	output := ExecuteCommand(pushCmd, []string{
		"--registry", "registry.example.com:5000",
		"--input-dir", "/tmp/nginx-mirror",
		"--concurrency", "8",
		"--verbose",
	})
	if output.Err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", output.Err, output.Stderr)
	}
}

// TestPushCmd_HelpFlag tests that push command supports --help flag.
func TestPushCmd_HelpFlag(t *testing.T) {
	output := ExecuteCommand(pushCmd, []string{"--help"})
	if output.Err != nil {
		t.Fatalf("--help should not produce error: %v", output.Err)
	}
	// Help output should contain command description
	helpOutput := output.Stdout + output.Stderr
	if !containsSubstring(helpOutput, "Push") || !containsSubstring(helpOutput, "images") {
		t.Fatalf("help output missing expected content: %s", helpOutput)
	}
}

// TestPushCmd_UnknownFlag tests that unknown flags are rejected.
func TestPushCmd_UnknownFlag(t *testing.T) {
	output := ExecuteCommand(pushCmd, []string{
		"--registry", "docker.io",
		"--unknown-flag", "value",
	})
	if output.Err == nil {
		t.Fatalf("expected error for unknown flag, got none")
	}
}

// Stub to allow command testing without external dependencies.
var _ context.Context
