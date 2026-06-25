package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"helm-deep-pack/internal/pull"
	"helm-deep-pack/internal/validation"
)

var (
	pullChart       string
	pullRepo        string
	pullVersion     string
	pullOutputDir   string
	pullConcurrency int
	pullAllowHTTP   bool
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
				if err := validation.ValidateURL("--repo", pullRepo, pullAllowHTTP); err != nil {
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
	pullCmd.Flags().StringVarP(&pullRepo, "repo", "r", "", "Helm repository URL (https or oci:// by default; optional; if not provided, searches configured Helm repositories)")
	pullCmd.Flags().StringVarP(&pullVersion, "version", "v", "", "Helm chart version")
	pullCmd.Flags().StringVarP(&pullOutputDir, "output-dir", "o", "", "Directory for OCI layout artifacts and script")
	pullCmd.Flags().IntVarP(&pullConcurrency, "concurrency", "c", 4, "Number of images to fetch and stage concurrently")
	pullCmd.Flags().BoolVarP(&pullAllowHTTP, "allow-insecure-http", "k", false, "Allow plaintext HTTP for Helm repository URLs")
	pullCmd.Flags().BoolVarP(&pullVerbose, "verbose", "V", false, "Enable verbose logging")
}

func isOCIReference(value string) bool {
	return strings.HasPrefix(strings.ToLower(value), "oci://")
}
