package cmd

import (
	"github.com/spf13/cobra"

	"helm-deep-pack/internal/add"
	"helm-deep-pack/internal/validation"
)

var (
	addImages      []string
	addOutputDir   string
	addConcurrency int
	addVerbose     bool
)

var addRun = add.Run

var addCmd = &cobra.Command{
	Use:   "add IMAGE...",
	Short: "Add images manually to an existing OCI layout",
	Long:  "Add extra container images to an existing pull output directory (run pull first). Appends images into the OCI layout and updates the push manifest.",
	Args:  cobra.MinimumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		addImages = args

		for _, img := range addImages {
			if err := validation.ValidateImage("image argument", img); err != nil {
				return err
			}
		}
		if err := validation.ValidateConcurrency("--concurrency", addConcurrency); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		commandLogger(addVerbose).Debug("adding images",
			"images", len(addImages),
			"concurrency", addConcurrency,
		)

		return addRun(cmd.Context(), add.Options{
			OutputDir:   addOutputDir,
			Images:      addImages,
			Concurrency: addConcurrency,
		}, cmd.ErrOrStderr())
	},
}

func init() {
	addCmd.Flags().StringVarP(&addOutputDir, "output-dir", "o", "", "Directory for OCI layout artifacts and script")
	addCmd.Flags().IntVarP(&addConcurrency, "concurrency", "c", 4, "Number of images to fetch and stage concurrently")
	addCmd.Flags().BoolVarP(&addVerbose, "verbose", "V", false, "Enable verbose logging")
}
