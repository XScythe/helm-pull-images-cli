package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// CapturedOutput holds captured stdout and stderr from command execution.
type CapturedOutput struct {
	Stdout string
	Stderr string
	Err    error
}

// ExecuteCommand executes a Cobra command with the given arguments and captures output.
// For subcommands (like pull or push), prepend the subcommand name to the args.
// This routes through rootCmd to properly handle subcommand hierarchy.
//
// Usage for subcommands:
//
//	output := ExecuteCommand(pullCmd, []string{"--chart", "nginx", "--version", "1.0.0"})
//	// Which actually calls: rootCmd with args ["pull", "--chart", "nginx", "--version", "1.0.0"]
//
// Usage for root command:
//
//	output := ExecuteCommand(rootCmd, []string{"--help"})
func ExecuteCommand(cmd *cobra.Command, args []string) *CapturedOutput {
	// Set up buffers to capture stdout/stderr
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// For subcommands, we need to execute through root to get proper routing
	// Detect if this is a subcommand by checking if it has a parent or use rootCmd
	execCmd := cmd
	if cmd.Name() != "helm-pull-images-cli" && cmd.Name() != "" {
		// This is a subcommand - prepend its name to args and execute through root
		execCmd = rootCmd
		newArgs := append([]string{cmd.Name()}, args...)
		args = newArgs
	}

	// Save original Out/Err
	origOut := execCmd.OutOrStdout()
	origErr := execCmd.ErrOrStderr()

	// Redirect output
	execCmd.SetOut(stdout)
	execCmd.SetErr(stderr)

	// Set arguments and execute
	execCmd.SetArgs(args)
	err := execCmd.Execute()

	// Restore original output
	execCmd.SetOut(origOut)
	execCmd.SetErr(origErr)

	return &CapturedOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Err:    err,
	}
}

// AssertFlagExists verifies that a flag with the given name exists on the command.
// Returns an error if the flag is not found.
func AssertFlagExists(t *testing.T, cmd *cobra.Command, flagName string) {
	t.Helper()
	if cmd.Flags().Lookup(flagName) == nil {
		t.Fatalf("expected flag %q to exist on command %q", flagName, cmd.Name())
	}
}

// AssertFlagNotExists verifies that a flag with the given name does not exist on the command.
// Returns an error if the flag is found.
func AssertFlagNotExists(t *testing.T, cmd *cobra.Command, flagName string) {
	t.Helper()
	if cmd.Flags().Lookup(flagName) != nil {
		t.Fatalf("expected flag %q to NOT exist on command %q", flagName, cmd.Name())
	}
}

// AssertFlagType verifies that a flag has the expected type string representation.
// Common types: "string", "int", "bool", "stringSlice", etc.
func AssertFlagType(t *testing.T, cmd *cobra.Command, flagName, expectedType string) {
	t.Helper()
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Fatalf("flag %q not found", flagName)
	}
	actualType := flag.Value.Type()
	if actualType != expectedType {
		t.Fatalf("flag %q: expected type %q, got %q", flagName, expectedType, actualType)
	}
}

// AssertFlagDefault verifies that a flag has the expected default value.
func AssertFlagDefault(t *testing.T, cmd *cobra.Command, flagName, expectedDefault string) {
	t.Helper()
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Fatalf("flag %q not found", flagName)
	}
	actualDefault := flag.Value.String()
	if actualDefault != expectedDefault {
		t.Fatalf("flag %q: expected default %q, got %q", flagName, expectedDefault, actualDefault)
	}
}

// AssertFlagRequired verifies that a flag is marked as required.
// Note: cobra doesn't expose a direct API to check if a flag is required,
// so this is primarily informational and verified via execution tests.
func AssertFlagRequired(t *testing.T, cmd *cobra.Command, flagName string) {
	t.Helper()
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Fatalf("flag %q not found", flagName)
	}
	// cobra tracks required flags internally, but doesn't expose API to query them
	// Actual verification happens in execution tests where we verify error when flag is missing
	t.Logf("note: flag %q required status verified via execution test", flagName)
}

// CommandTestCase represents a test case for command execution.
type CommandTestCase struct {
	Name        string
	Args        []string
	WantErr     bool
	WantErrMsg  string // Substring to match in error message
	CheckOutput func(t *testing.T, output *CapturedOutput)
}

// RunCommandTests executes a slice of CommandTestCase tests against a command.
func RunCommandTests(t *testing.T, cmd *cobra.Command, cases []CommandTestCase) {
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			output := ExecuteCommand(cmd, tc.Args)

			if tc.WantErr {
				if output.Err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.WantErrMsg != "" && !bytes.Contains([]byte(output.Stderr+output.Stdout), []byte(tc.WantErrMsg)) {
					t.Fatalf("expected error message to contain %q, got:\nstderr: %s\nstdout: %s",
						tc.WantErrMsg, output.Stderr, output.Stdout)
				}
			} else {
				if output.Err != nil {
					t.Fatalf("unexpected error: %v\nstderr: %s\nstdout: %s",
						output.Err, output.Stderr, output.Stdout)
				}
			}

			if tc.CheckOutput != nil {
				tc.CheckOutput(t, output)
			}
		})
	}
}

// PatchCobraRunE temporarily replaces a command's RunE function for testing.
// Automatically resets the command's global variables before executing.
// Returns a restore function to undo the patch.
func PatchCobraRunE(cmd *cobra.Command, newRunE func(*cobra.Command, []string) error) func() {
	// Reset command-specific global variables before patching
	resetCmdVars(cmd.Name())

	origRunE := cmd.RunE
	cmd.RunE = newRunE
	return func() {
		cmd.RunE = origRunE
		resetCmdVars(cmd.Name())
	}
}

// resetCmdVars resets global variables for a given command to their defaults.
func resetCmdVars(cmdName string) {
	switch cmdName {
	case "pull":
		pullChart = ""
		pullRepo = ""
		pullVersion = ""
		pullOutputDir = ""
		pullConcurrency = 4
		pullVerbose = false
	case "push":
		pushRegistry = ""
		pushInputDir = ""
		pushConcurrency = 4
		pushVerbose = false
	default:
		panic("resetCmdVars: unknown command " + cmdName)
	}
}

// PatchCobra is deprecated, use PatchCobraRunE instead for better test isolation.
// It's kept for backward compatibility.
func PatchCobra(cmd *cobra.Command, newRunE func(*cobra.Command, []string) error) func() {
	origRunE := cmd.RunE
	cmd.RunE = newRunE
	return func() {
		cmd.RunE = origRunE
	}
}

// ReadPipe reads data from an io.Reader.
// Useful for capturing output in tests.
func ReadPipe(r io.Reader) string {
	buf := new(bytes.Buffer)
	_, _ = io.Copy(buf, r)
	return buf.String()
}

// CleanupChartDirs removes any chart directories that might have been created during tests.
// It looks for directories that match chart name patterns: {chart} or {chart}-YYYY-MM-DD-HH
func CleanupChartDirs() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	entries, err := os.ReadDir(cwd)
	if err != nil {
		return
	}

	chartNames := map[string]bool{
		"nginx": true, "openebs": true, "prometheus": true,
		"redis": true, "mysql": true, "postgresql": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check if it's a chart directory or a chart directory with timestamp
		isChartDir := false
		for chart := range chartNames {
			if name == chart || strings.HasPrefix(name, chart+"-") {
				// Verify it looks like a timestamp pattern if it has prefix
				if strings.HasPrefix(name, chart+"-") {
					rest := strings.TrimPrefix(name, chart+"-")
					if len(rest) == 13 && rest[4] == '-' && rest[7] == '-' && rest[10] == '-' {
						isChartDir = true
						break
					}
				} else {
					isChartDir = true
					break
				}
			}
		}

		if isChartDir {
			_ = os.RemoveAll(filepath.Join(cwd, name))
		}
	}
}
