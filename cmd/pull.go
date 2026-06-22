package cmd

import (
	"helm-pull-images-cli/internal/pull"
	"helm-pull-images-cli/internal/validation"

	"github.com/spf13/cobra"
)

var (
	pullChart       string
	pullRepo        string
	pullVersion     string
	pullOutputDir   string
	pullConcurrency int
	pullVerbose     bool
)

var pullCmd = &cobra.Command{
	Use:   "pull CHART",
	Short: "Render a chart and mirror its images",
	Long:  "Render a Helm chart and extract all referenced container images. Requires either --repo or locally configured Helm repositories.",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		pullChart = args[0]
		// Validate all flags
		if err := validation.ValidateChartName("chart argument", pullChart); err != nil {
			return err
		}
		if pullRepo != "" {
			if err := validation.ValidateURL("--repo", pullRepo); err != nil {
				return err
			}
		}
		if err := validation.ValidateVersion("--version", pullVersion); err != nil {
			return err
		}
		if err := validation.ValidateConcurrency("--concurrency", pullConcurrency); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		commandLogger(pullVerbose).Debug("pulling chart",
			"chart", pullChart,
			"repo", pullRepo,
			"version", pullVersion,
			"concurrency", pullConcurrency,
		)

		return pull.Run(cmd.Context(), pull.Options{
			Chart:       pullChart,
			Repo:        pullRepo,
			Version:     pullVersion,
			OutputDir:   pullOutputDir,
			Concurrency: pullConcurrency,
		}, cmd.ErrOrStderr())
	},
}

func init() {
	pullCmd.Flags().StringVar(&pullRepo, "repo", "", "Helm repository URL (optional; if not provided, searches configured Helm repositories)")
	pullCmd.Flags().StringVar(&pullVersion, "version", "", "Helm chart version")
	pullCmd.Flags().StringVar(&pullOutputDir, "output-dir", "", "Directory for OCI layout artifacts and script")
	pullCmd.Flags().IntVar(&pullConcurrency, "concurrency", 4, "Number of images to fetch and stage concurrently")
	pullCmd.Flags().BoolVar(&pullVerbose, "verbose", false, "Enable verbose logging")
}
