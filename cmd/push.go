package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"helm-pull-images-cli/internal/mirror"
)

var (
	pushRegistry string
	pushInputDir string
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push mirrored images from generated archives",
	RunE: func(cmd *cobra.Command, args []string) error {
		if pushRegistry == "" {
			return fmt.Errorf("--registry is required")
		}

		inputDir := pushInputDir
		if inputDir == "" {
			dir, err := defaultPushInputDir()
			if err != nil {
				return err
			}
			inputDir = dir
		}

		return mirror.PushImages(cmd.Context(), pushRegistry, inputDir)
	},
}

func init() {
	pushCmd.Flags().StringVar(&pushRegistry, "registry", "", "Target registry host")
	pushCmd.Flags().StringVar(&pushInputDir, "input-dir", "", "Directory containing push_images.json and archives (defaults to the helper binary directory)")
}

func defaultPushInputDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Dir(executable), nil
}
