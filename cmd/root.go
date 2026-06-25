package cmd

import (
	"helm-deep-pack/internal/upgrade"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

// commandLogger returns a stderr logger; verbose enables debug-level output.
func commandLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "helm-deep-pack",
	Short:   "Mirror Helm chart images into OCI layout artifacts",
	Version: version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	upgrade.CleanupStale()
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func Version() string {
	return version
}

func init() {
	rootCmd.SetVersionTemplate("helm-deep-pack {{.Version}}\n")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(upgradeHelperCmd)
}
