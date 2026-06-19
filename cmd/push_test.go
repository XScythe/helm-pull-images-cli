package cmd

import "testing"

func TestPushCommandExposesRegistryFlag(t *testing.T) {
	if pushCmd.Flags().Lookup("registry") == nil {
		t.Fatalf("push command missing registry flag")
	}
}

func TestPushCommandExposesConcurrencyFlag(t *testing.T) {
	if pushCmd.Flags().Lookup("concurrency") == nil {
		t.Fatalf("push command missing concurrency flag")
	}
}
