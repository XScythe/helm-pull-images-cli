package cmd

import (
	"fmt"
	"github.com/spf13/cobra"

	"helm-deep-pack/internal/upgrade"
)

var (
	upgradeHelperTargetExe   string
	upgradeHelperIncomingExe string
)

var upgradeHelperCmd = &cobra.Command{
	Use:          "upgrade-helper",
	Hidden:       true,
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if upgradeHelperTargetExe == "" {
			return fmt.Errorf("missing --target-exe")
		}
		if upgradeHelperIncomingExe == "" {
			return fmt.Errorf("missing --incoming-exe")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return upgrade.RunHelper(cmd.Context(), upgradeHelperTargetExe, upgradeHelperIncomingExe)
	},
}

func init() {
	upgradeHelperCmd.Flags().StringVar(&upgradeHelperTargetExe, "target-exe", "", "Target executable path")
	upgradeHelperCmd.Flags().StringVar(&upgradeHelperIncomingExe, "incoming-exe", "", "Incoming executable path")
}
