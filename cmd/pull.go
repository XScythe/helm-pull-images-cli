package cmd

import (
	"fmt"
	"helm-pull-images-cli/internal/pull"
	"helm-pull-images-cli/internal/validation"
	"strings"

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

var pullRun = pull.Run

var pullCmd = &cobra.Command{
	Use:   "pull CHART",
	Short: "Render a chart and mirror its images",
	Long:  "Render a Helm chart and extract all referenced container images. Supports local charts, HTTP(S) repositories, configured Helm repositories, and oci:// chart references.",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		pullChart = args[0]

		chartIsOCI := isOCIReference(pullChart)
		repoIsOCI := isOCIReference(pullRepo)

		if chartIsOCI {
			if err := validation.ValidateOCIRef("chart argument", pullChart); err != nil {
				return err
			}
			if pullRepo != "" {
				return fmt.Errorf("--repo cannot be combined with an oci:// chart reference")
			}
		} else if err := validation.ValidateChartName("chart argument", pullChart); err != nil {
			return err
		}

		if pullRepo != "" {
			if repoIsOCI {
				if err := validation.ValidateOCIRef("--repo", pullRepo); err != nil {
					return err
				}
			} else {
				if err := validation.ValidateURL("--repo", pullRepo); err != nil {
					return err
				}
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

		return pullRun(cmd.Context(), pull.Options{
			Chart:       pullChart,
			Repo:        pullRepo,
			Version:     pullVersion,
			OutputDir:   pullOutputDir,
			Concurrency: pullConcurrency,
		}, cmd.ErrOrStderr())
	},
}

func init() {
	pullCmd.Flags().StringVar(&pullRepo, "repo", "", "Helm repository URL (http/https or oci://; optional; if not provided, searches configured Helm repositories)")
	pullCmd.Flags().StringVar(&pullVersion, "version", "", "Helm chart version")
	pullCmd.Flags().StringVar(&pullOutputDir, "output-dir", "", "Directory for OCI layout artifacts and script")
	pullCmd.Flags().IntVar(&pullConcurrency, "concurrency", 4, "Number of images to fetch and stage concurrently")
	pullCmd.Flags().BoolVar(&pullVerbose, "verbose", false, "Enable verbose logging")
}

func isOCIReference(value string) bool {
	return strings.HasPrefix(strings.ToLower(value), "oci://")
}
