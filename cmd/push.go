package cmd

import (
	"github.com/spf13/cobra"
	"helm-pull-images-cli/internal/config"
	"helm-pull-images-cli/internal/mirror"
	"helm-pull-images-cli/internal/validation"
)

var (
	pushRegistry    string
	pushInputDir    string
	pushConcurrency int
	pushVerbose     bool
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push mirrored images from generated OCI layout artifacts",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Validate all flags
		if err := validation.ValidateImageRegistry("--registry", pushRegistry); err != nil {
			return err
		}
		if err := validation.ValidateConcurrency("--concurrency", pushConcurrency); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.New().WithVerbose(pushVerbose)
		cfg.Logger.Info("pushing images to registry",
			"registry", pushRegistry,
			"concurrency", pushConcurrency,
		)

		return mirror.PushImages(cmd.Context(), pushRegistry, pushInputDir, pushConcurrency)
	},
}

func init() {
	pushCmd.Flags().StringVar(&pushRegistry, "registry", "", "Target registry host")
	pushCmd.Flags().StringVar(&pushInputDir, "input-dir", "", "Directory containing push_images.json and OCI layout artifacts")
	pushCmd.Flags().IntVar(&pushConcurrency, "concurrency", 4, "Number of images to push concurrently")
	pushCmd.Flags().BoolVar(&pushVerbose, "verbose", false, "Enable verbose logging")
	pushCmd.MarkFlagRequired("registry")
}
