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

func TestSharepointSitesGetRequiresSite(t *testing.T) {
	if sharepointSitesGetCmd.Flags().Lookup("site") == nil {
		t.Fatal("sharepoint sites get missing --site flag")
	}
}

func TestSharepointListsListRequiresSite(t *testing.T) {
	if sharepointListsListCmd.Flags().Lookup("site") == nil {
		t.Fatal("sharepoint lists list missing --site flag")
	}
}

func TestSharepointListsItemsListRequiresSiteAndList(t *testing.T) {
	if sharepointListsItemsListCmd.Flags().Lookup("site") == nil {
		t.Fatal("missing --site")
	}
	if sharepointListsItemsListCmd.Flags().Lookup("list") == nil {
		t.Fatal("missing --list")
	}
}

func TestSharepointListsItemsCreateRequiresFields(t *testing.T) {
	if sharepointListsItemsCreateCmd.Flags().Lookup("site") == nil {
		t.Fatal("missing --site")
	}
	if sharepointListsItemsCreateCmd.Flags().Lookup("list") == nil {
		t.Fatal("missing --list")
	}
	if sharepointListsItemsCreateCmd.Flags().Lookup("field") == nil {
		t.Fatal("missing --field")
	}
}

func TestSharepointListsItemsUpdateRequiresItem(t *testing.T) {
	if sharepointListsItemsUpdateCmd.Flags().Lookup("item") == nil {
		t.Fatal("missing --item")
	}
	if sharepointListsItemsUpdateCmd.Flags().Lookup("field") == nil {
		t.Fatal("missing --field")
	}
}

func TestSharepointListsItemsDeleteRequiresForce(t *testing.T) {
	f := sharepointListsItemsDeleteCmd.Flags().Lookup("force")
	if f == nil {
		t.Fatal("missing --force")
	}
	if f.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", f.DefValue)
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

func TestSharepointFilesUploadSafetyFlags(t *testing.T) {
	if sharepointFilesUploadCmd.Flags().Lookup("force") == nil {
		t.Fatal("missing --force on upload")
	}
	if sharepointFilesUploadCmd.Flags().Lookup("file") == nil {
		t.Fatal("missing --file on upload")
	}
	if sharepointFilesUploadCmd.Flags().Lookup("path") == nil {
		t.Fatal("missing --path on upload")
	}
}

func TestSharepointFilesGetSafetyFlags(t *testing.T) {
	if sharepointFilesGetCmd.Flags().Lookup("output") == nil {
		t.Fatal("missing --output on get")
	}
	if sharepointFilesGetCmd.Flags().Lookup("force") == nil {
		t.Fatal("missing --force on get")
	}
}

func TestSharepointSitesListSearchFlag(t *testing.T) {
	if sharepointSitesListCmd.Flags().Lookup("search") == nil {
		t.Fatal("missing --search")
	}
}

func TestFormatSiteURL(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"https://contoso.sharepoint.com", "https://contoso.sharepoint.com"},
		{"", "(unknown)"},
	}
	for _, tt := range tests {
		if result := formatSiteURL(tt.input); result != tt.expected {
			t.Errorf("formatSiteURL(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

