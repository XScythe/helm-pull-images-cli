package cmd

import (
	"github.com/spf13/cobra"

	"helm-deep-pack/internal/push"
	"helm-deep-pack/internal/validation"
)

var (
	pushRegistry    string
	pushInputDir    string
	pushConcurrency int
	pushAll         bool
	pushVerbose     bool
)

var pushRun = push.PushImages

var pushCmd = &cobra.Command{
	Use:   "push REGISTRY",
	Short: "Push mirrored images from generated OCI layout artifacts",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		pushRegistry = args[0]
		// Validate all flags
		if err := validation.ValidateImageRegistry("registry argument", pushRegistry); err != nil {
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

		return pushRun(cmd.Context(), push.Options{
			Registry:    pushRegistry,
			InputDir:    pushInputDir,
			Concurrency: pushConcurrency,
			All:         pushAll,
			In:          cmd.InOrStdin(),
			Out:         cmd.OutOrStdout(),
		}, cmd.ErrOrStderr())
	},
}

func init() {
	pushCmd.Flags().StringVar(&pushInputDir, "input-dir", "", "Directory containing push_images.json and OCI layout artifacts")
	pushCmd.Flags().IntVar(&pushConcurrency, "concurrency", 4, "Number of images to push concurrently")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "Push all images without interactive selection")
	pushCmd.Flags().BoolVar(&pushVerbose, "verbose", false, "Enable verbose logging")
}
