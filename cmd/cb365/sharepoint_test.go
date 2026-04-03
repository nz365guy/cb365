package main

import (
	"testing"
)

// ──────────────────────────────────────────────
//  SharePoint safety and structure tests
// ──────────────────────────────────────────────

func TestSharepointCommandStructure(t *testing.T) {
	if !sharepointCmd.HasSubCommands() {
		t.Fatal("sharepoint command should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range sharepointCmd.Commands() {
		found[sub.Name()] = true
	}

	for _, expected := range []string{"sites", "lists"} {
		if !found[expected] {
			t.Errorf("sharepoint missing subcommand %q", expected)
		}
	}
}

func TestSharepointAliases(t *testing.T) {
	if len(sharepointCmd.Aliases) == 0 {
		t.Fatal("sharepoint should have alias 'sp'")
	}
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

func TestSharepointSitesGetRequiresSite(t *testing.T) {
	cmd := sharepointSitesGetCmd
	siteFlag := cmd.Flags().Lookup("site")
	if siteFlag == nil {
		t.Fatal("sharepoint sites get missing --site flag")
	}
}

func TestSharepointListsListRequiresSite(t *testing.T) {
	cmd := sharepointListsListCmd
	siteFlag := cmd.Flags().Lookup("site")
	if siteFlag == nil {
		t.Fatal("sharepoint lists list missing --site flag")
	}
}

func TestSharepointListsItemsRequiresSiteAndList(t *testing.T) {
	cmd := sharepointListsItemsCmd
	siteFlag := cmd.Flags().Lookup("site")
	if siteFlag == nil {
		t.Fatal("sharepoint lists items missing --site flag")
	}
	listFlag := cmd.Flags().Lookup("list")
	if listFlag == nil {
		t.Fatal("sharepoint lists items missing --list flag")
	}
}

func TestSharepointListsItemsMaxDefault(t *testing.T) {
	cmd := sharepointListsItemsCmd
	maxFlag := cmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Fatal("sharepoint lists items missing --max flag")
	}
	if maxFlag.DefValue != "50" {
		t.Errorf("--max default should be 50, got %s", maxFlag.DefValue)
	}
}

func TestSharepointSitesListSearchFlag(t *testing.T) {
	cmd := sharepointSitesListCmd
	searchFlag := cmd.Flags().Lookup("search")
	if searchFlag == nil {
		t.Fatal("sharepoint sites list missing --search flag")
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

