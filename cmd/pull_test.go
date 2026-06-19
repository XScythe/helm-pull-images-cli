package cmd

import "testing"

func TestPullCommandDoesNotExposeRegistryFlag(t *testing.T) {
	if pullCmd.Flags().Lookup("registry") != nil {
		t.Fatalf("pull command unexpectedly exposes a registry flag")
	}
}

func TestPullCommandExposesLocalFlag(t *testing.T) {
	if pullCmd.Flags().Lookup("local") == nil {
		t.Fatalf("pull command missing local flag")
	}
}
