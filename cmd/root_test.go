package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRootCmd_HasSubcommands verifies root command has the expected subcommands.
func TestRootCmd_HasSubcommands(t *testing.T) {
	commands := []string{"pull", "push", "upgrade"}
	for _, cmdName := range commands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected subcommand %q not found in root command", cmdName)
		}
	}
}

// TestRootCmd_HelpFlag tests that root command supports --help flag.
func TestRootCmd_HelpFlag(t *testing.T) {
	output := ExecuteCommand(rootCmd, []string{"--help"})
	if output.Err != nil {
		t.Fatalf("--help should not produce error: %v", output.Err)
	}
	// Help output should mention subcommands
	helpOutput := output.Stdout + output.Stderr
	if !strings.Contains(helpOutput, "pull") || !strings.Contains(helpOutput, "push") || !strings.Contains(helpOutput, "upgrade") {
		t.Fatalf("help output missing subcommand mentions: %s", helpOutput)
	}
}

// TestRootCmd_InvalidSubcommand tests that invalid subcommands are rejected.
func TestRootCmd_InvalidSubcommand(t *testing.T) {
	output := ExecuteCommand(rootCmd, []string{"invalid-command"})
	if output.Err == nil {
		t.Fatalf("expected error for invalid subcommand, got none")
	}
	if !strings.Contains(output.Stderr, "Run 'helm-deep-pack --help' for usage.") {
		t.Fatalf("expected usage hint for invalid subcommand, got: %s", output.Stderr)
	}
}

// TestRootCmd_NoArgs verifies that root command without args produces error or help.
func TestRootCmd_NoArgs(t *testing.T) {
	output := ExecuteCommand(rootCmd, []string{})
	// Root command without args should produce an error or show usage
	errOutput := output.Stderr + output.Stdout
	if output.Err == nil && len(errOutput) == 0 {
		t.Fatalf("expected error or help output when no args provided, got neither")
	}
}

func TestRootCmd_SilencesUsageForCommandErrors(t *testing.T) {
	tempCmd := &cobra.Command{
		Use: "temp",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("boom")
		},
	}
	rootCmd.AddCommand(tempCmd)
	defer rootCmd.RemoveCommand(tempCmd)

	output := ExecuteCommand(tempCmd, []string{})
	if output.Err == nil {
		t.Fatalf("expected error for temp command, got none")
	}
	if strings.Contains(output.Stderr, "Usage:") || strings.Contains(output.Stderr, "Run 'helm-deep-pack --help' for usage.") {
		t.Fatalf("expected no usage output for command error, got: %s", output.Stderr)
	}
}

// TestRootCmd_PullSubcommand tests that 'pull' subcommand can be invoked.
func TestRootCmd_PullSubcommand(t *testing.T) {
	// Test that 'pull --help' works from root
	output := ExecuteCommand(rootCmd, []string{"pull", "--help"})
	if output.Err != nil {
		t.Fatalf("pull subcommand --help failed: %v", output.Err)
	}
	helpOutput := output.Stdout + output.Stderr
	if !strings.Contains(helpOutput, "chart") {
		t.Fatalf("pull help missing 'chart' flag: %s", helpOutput)
	}
}

// TestRootCmd_PushSubcommand tests that 'push' subcommand can be invoked.
func TestRootCmd_PushSubcommand(t *testing.T) {
	// Test that 'push --help' works from root
	output := ExecuteCommand(rootCmd, []string{"push", "--help"})
	if output.Err != nil {
		t.Fatalf("push subcommand --help failed: %v", output.Err)
	}
	helpOutput := output.Stdout + output.Stderr
	if !strings.Contains(helpOutput, "REGISTRY") {
		t.Fatalf("push help missing 'REGISTRY' argument: %s", helpOutput)
	}
}

func TestRootCmd_UpgradeSubcommand(t *testing.T) {
	output := ExecuteCommand(rootCmd, []string{"upgrade", "--help"})
	if output.Err != nil {
		t.Fatalf("upgrade subcommand --help failed: %v", output.Err)
	}
	helpOutput := output.Stdout + output.Stderr
	if !strings.Contains(helpOutput, "latest stable") {
		t.Fatalf("upgrade help missing expected text: %s", helpOutput)
	}
}

func TestRootCmd_VersionFlag(t *testing.T) {
	output := ExecuteCommand(rootCmd, []string{"--version"})
	if output.Err != nil {
		t.Fatalf("--version should not produce error: %v", output.Err)
	}
	combined := output.Stdout + output.Stderr
	if !strings.Contains(combined, "helm-deep-pack") {
		t.Fatalf("expected version output to include binary name, got: %s", combined)
	}
}

func TestRunPushHelperIfNeeded_AllowsZeroArgsAndReachesWorkflow(t *testing.T) {
	// Regression: the standalone helper must allow zero args so a no-arg launch
	// (e.g. double-click) reaches the prompt-capable workflow instead of Cobra's
	// ExactArgs "accepts 1 arg(s), received 0" rejection.
	capture, restore := spyPushRun(nil)
	defer restore()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	handled, err := runPushHelperIfNeeded([]string{"push_images"}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("runPushHelperIfNeeded() error = %v", err)
	}
	if !handled {
		t.Fatalf("runPushHelperIfNeeded() handled = false, want true")
	}
	if !capture.called {
		t.Fatalf("expected push workflow to be reached with zero args")
	}
	if capture.opts.Registry != "" {
		t.Fatalf("expected empty registry to be passed through for prompting, got %q", capture.opts.Registry)
	}
	combined := strings.ToLower(stdout.String() + stderr.String())
	if strings.Contains(combined, "accepts 1 arg") {
		t.Fatalf("regressed to ExactArgs rejection: %s", combined)
	}
}

func TestIsPushHelperInvocation(t *testing.T) {
	tests := []struct {
		name  string
		argv0 string
		want  bool
	}{
		{name: "exact name", argv0: "push_images", want: true},
		{name: "with path", argv0: "/tmp/work/push_images", want: true},
		{name: "with windows extension", argv0: `C:\tmp\push_images.exe`, want: true},
		{name: "normal binary", argv0: "helm-deep-pack", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPushHelperInvocation(tc.argv0); got != tc.want {
				t.Fatalf("isPushHelperInvocation(%q) = %v, want %v", tc.argv0, got, tc.want)
			}
		})
	}
}

func TestRunPushHelperIfNeeded_InvokesPushWorkflow(t *testing.T) {
	capture, restore := spyPushRun(nil)
	defer restore()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	handled, err := runPushHelperIfNeeded([]string{"push_images", "docker.io", "-a"}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("runPushHelperIfNeeded() error = %v", err)
	}
	if !handled {
		t.Fatalf("runPushHelperIfNeeded() handled = false, want true")
	}
	if !capture.called {
		t.Fatalf("expected push workflow to be called")
	}
	if capture.opts.Registry != "docker.io" || !capture.opts.All {
		t.Fatalf("unexpected options: %#v", capture.opts)
	}
}

func TestRunPushHelperIfNeeded_AcceptsLegacyPushSubcommand(t *testing.T) {
	capture, restore := spyPushRun(nil)
	defer restore()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	handled, err := runPushHelperIfNeeded([]string{"push_images", "push", "docker.io", "--all"}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("runPushHelperIfNeeded() error = %v", err)
	}
	if !handled {
		t.Fatalf("runPushHelperIfNeeded() handled = false, want true")
	}
	if !capture.called {
		t.Fatalf("expected push workflow to be called")
	}
	if capture.opts.Registry != "docker.io" || !capture.opts.All {
		t.Fatalf("unexpected options: %#v", capture.opts)
	}
}
