package main

import (
	"testing"
)

// ──────────────────────────────────────────────
//  OneDrive safety and structure tests
// ──────────────────────────────────────────────

func TestOnedriveCommandStructure(t *testing.T) {
	if !onedriveCmd.HasSubCommands() {
		t.Fatal("onedrive command should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range onedriveCmd.Commands() {
		found[sub.Name()] = true
	}

	for _, expected := range []string{"ls", "get", "upload"} {
		if !found[expected] {
			t.Errorf("onedrive missing subcommand %q", expected)
		}
	}
}

func TestOnedriveAliases(t *testing.T) {
	if len(onedriveCmd.Aliases) == 0 {
		t.Fatal("onedrive should have alias 'od'")
	}
	found := false
	for _, a := range onedriveCmd.Aliases {
		if a == "od" {
			found = true
		}
	}
	if !found {
		t.Error("onedrive missing alias 'od'")
	}
}

func TestOnedriveGetRequiresOutput(t *testing.T) {
	cmd := onedriveGetCmd
	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("onedrive get missing --output flag")
	}
}

func TestOnedriveGetForceDefault(t *testing.T) {
	cmd := onedriveGetCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive get missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestOnedriveUploadRequiresFileAndPath(t *testing.T) {
	cmd := onedriveUploadCmd
	fileFlag := cmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Fatal("onedrive upload missing --file flag")
	}
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive upload missing --path flag")
	}
}

func TestOnedriveUploadForceDefault(t *testing.T) {
	cmd := onedriveUploadCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive upload missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestHumanFileSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{4194304, "4.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		result := humanFileSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("humanFileSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestOnedriveLsPathAndItemIDFlags(t *testing.T) {
	cmd := onedriveLsCmd
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive ls missing --path flag")
	}
	itemIDFlag := cmd.Flags().Lookup("item-id")
	if itemIDFlag == nil {
		t.Fatal("onedrive ls missing --item-id flag")
	}
}


func TestOnedriveDeleteRequiresForce(t *testing.T) {
	cmd := onedriveDeleteCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive delete missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestOnedriveMkdirRequiresPath(t *testing.T) {
	cmd := onedriveMkdirCmd
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive mkdir missing --path flag")
	}
}

func TestOnedriveCommandStructureWithNewCmds(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range onedriveCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"ls", "get", "upload", "delete", "mkdir"} {
		if !found[expected] {
			t.Errorf("onedrive missing subcommand %q", expected)
		}
	}
}
