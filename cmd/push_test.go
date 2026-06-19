package cmd

import "testing"

func TestPushCommandExposesRegistryFlag(t *testing.T) {
	if pushCmd.Flags().Lookup("registry") == nil {
		t.Fatalf("push command missing registry flag")
	}
}
