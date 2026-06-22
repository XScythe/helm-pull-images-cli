package cmd

import (
	"github.com/spf13/cobra"
	"helm-pull-images-cli/internal/chartmirror"
	"helm-pull-images-cli/internal/config"
	"helm-pull-images-cli/internal/validation"
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
	Use:   "pull",
	Short: "Render a chart and mirror its images",
	Long:  "Render a Helm chart and extract all referenced container images. Requires either --repo or locally configured Helm repositories.",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Validate all flags
		if err := validation.ValidateChartName("--chart", pullChart); err != nil {
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
		cfg := config.New().WithVerbose(pullVerbose)
		cfg.Logger.Info("pulling chart",
			"chart", pullChart,
			"repo", pullRepo,
			"version", pullVersion,
			"concurrency", pullConcurrency,
		)

		return chartmirror.Run(cmd.Context(), chartmirror.Options{
			Chart:       pullChart,
			Repo:        pullRepo,
			Version:     pullVersion,
			OutputDir:   pullOutputDir,
			Concurrency: pullConcurrency,
		})
	},
}

func init() {
	pullCmd.Flags().StringVar(&pullChart, "chart", "", "Helm chart name")
	pullCmd.Flags().StringVar(&pullRepo, "repo", "", "Helm repository URL (optional; if not provided, searches configured Helm repositories)")
	pullCmd.Flags().StringVar(&pullVersion, "version", "", "Helm chart version")
	pullCmd.Flags().StringVar(&pullOutputDir, "output-dir", "", "Directory for OCI layout artifacts and script")
	pullCmd.Flags().IntVar(&pullConcurrency, "concurrency", 4, "Number of images to fetch and stage concurrently")
	pullCmd.Flags().BoolVar(&pullVerbose, "verbose", false, "Enable verbose logging")
	pullCmd.MarkFlagRequired("chart")
}
