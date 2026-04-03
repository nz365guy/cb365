package main

import (
	"testing"
)

func TestSharepointCommandStructure(t *testing.T) {
	if !sharepointCmd.HasSubCommands() {
		t.Fatal("sharepoint command should have subcommands")
	}
	found := map[string]bool{}
	for _, sub := range sharepointCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"sites", "lists", "files"} {
		if !found[expected] {
			t.Errorf("sharepoint missing subcommand %q", expected)
		}
	}
}

func TestSharepointAliases(t *testing.T) {
	found := false
	for _, a := range sharepointCmd.Aliases {
		if a == "sp" {
			found = true
		}
	}
	if !found {
		t.Error("sharepoint missing alias 'sp'")
	}
}

func TestSharepointListsItemsStructure(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range sharepointListsItemsCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"list", "create", "update", "delete"} {
		if !found[expected] {
			t.Errorf("sharepoint lists items missing subcommand %q", expected)
		}
	}
}

func TestSharepointFilesStructure(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range sharepointFilesCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"list", "get", "upload"} {
		if !found[expected] {
			t.Errorf("sharepoint files missing subcommand %q", expected)
		}
	}
}

func TestSharepointItemsDeleteRequiresForce(t *testing.T) {
	cmd := sharepointListsItemsDeleteCmd
	if cmd.Flags().Lookup("force") == nil {
		t.Fatal("sp lists items delete missing --force flag")
	}
	if cmd.Flags().Lookup("force").DefValue != "false" {
		t.Error("--force default should be false")
	}
}

func TestSharepointFilesUploadRequiresForce(t *testing.T) {
	cmd := sharepointFilesUploadCmd
	if cmd.Flags().Lookup("force") == nil {
		t.Fatal("sp files upload missing --force flag")
	}
}

func TestSharepointFilesGetRequiresOutput(t *testing.T) {
	cmd := sharepointFilesGetCmd
	if cmd.Flags().Lookup("output") == nil {
		t.Fatal("sp files get missing --output flag")
	}
}

func TestSharepointItemsCreateRequiresField(t *testing.T) {
	cmd := sharepointListsItemsCreateCmd
	if cmd.Flags().Lookup("field") == nil {
		t.Fatal("sp lists items create missing --field flag")
	}
}

func TestFormatSiteURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://cloverbase.sharepoint.com", "https://cloverbase.sharepoint.com"},
		{"", "(unknown)"},
	}
	for _, tt := range tests {
		result := formatSiteURL(tt.input)
		if result != tt.expected {
			t.Errorf("formatSiteURL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
