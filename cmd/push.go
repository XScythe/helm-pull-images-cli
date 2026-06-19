package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"helm-pull-images-cli/internal/mirror"
)

var (
	pushRegistry    string
	pushInputDir    string
	pushConcurrency int
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push mirrored images from generated OCI layout artifacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		if pushRegistry == "" {
			return fmt.Errorf("--registry is required")
		}

		return mirror.PushImages(cmd.Context(), pushRegistry, pushInputDir, pushConcurrency)
	},
}

func init() {
	pushCmd.Flags().StringVar(&pushRegistry, "registry", "", "Target registry host")
	pushCmd.Flags().StringVar(&pushInputDir, "input-dir", "", "Directory containing push_images.json and OCI layout artifacts (defaults to the helper binary directory)")
	pushCmd.Flags().IntVar(&pushConcurrency, "concurrency", 4, "Number of images to push concurrently")
}
