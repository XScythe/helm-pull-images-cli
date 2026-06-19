package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"helm-pull-images-cli/internal/chartmirror"
)

var (
	pullChart     string
	pullRepo      string
	pullVersion   string
	pullNamespace string
	pullOutputDir string
	pullRelease   string
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Render a chart and mirror its images",
	RunE: func(cmd *cobra.Command, args []string) error {
		if pullChart == "" {
			return fmt.Errorf("--chart is required")
		}

		return chartmirror.Run(cmd.Context(), chartmirror.Options{
			ReleaseName: pullRelease,
			Chart:       pullChart,
			Repo:        pullRepo,
			Version:     pullVersion,
			Namespace:   pullNamespace,
			OutputDir:   pullOutputDir,
		})
	},
}

func init() {
	pullCmd.Flags().StringVar(&pullChart, "chart", "", "Helm chart name")
	pullCmd.Flags().StringVar(&pullRepo, "repo", "", "Helm repository URL")
	pullCmd.Flags().StringVar(&pullVersion, "version", "", "Helm chart version")
	pullCmd.Flags().StringVar(&pullNamespace, "namespace", "default", "Release namespace")
	pullCmd.Flags().StringVar(&pullOutputDir, "output-dir", "", "Directory for archives and script (defaults to a new directory in the current working directory)")
	pullCmd.Flags().StringVar(&pullRelease, "release-name", "mirror", "Helm release name")
	_ = pullCmd.MarkFlagRequired("chart")
}
