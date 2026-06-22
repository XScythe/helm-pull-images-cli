package cmd

import (
	"helm-pull-images-cli/internal/push"
	"helm-pull-images-cli/internal/validation"

	"github.com/spf13/cobra"
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
		commandLogger(pushVerbose).Debug("pushing images to registry",
			"registry", pushRegistry,
			"concurrency", pushConcurrency,
		)

		return push.PushImages(cmd.Context(), pushRegistry, pushInputDir, pushConcurrency, cmd.ErrOrStderr())
	},
}

func init() {
	pushCmd.Flags().StringVar(&pushRegistry, "registry", "", "Target registry host")
	pushCmd.Flags().StringVar(&pushInputDir, "input-dir", "", "Directory containing push_images.json and OCI layout artifacts")
	pushCmd.Flags().IntVar(&pushConcurrency, "concurrency", 4, "Number of images to push concurrently")
	pushCmd.Flags().BoolVar(&pushVerbose, "verbose", false, "Enable verbose logging")
	pushCmd.MarkFlagRequired("registry")
}
