package main

import (
	"os"
	"testing"
)

// ─── Unit tests (no Graph API needed) ───

func TestContactEmailsString(t *testing.T) {
	// With nil input
	result := contactEmailsString(nil)
	if result != "" {
		t.Errorf("contactEmailsString(nil) = %q, want empty", result)
	}
}

func TestGetInternalDomainForContacts(t *testing.T) {
	// Verify the domain env var works (shared with mail)
	os.Setenv("CB365_INTERNAL_DOMAIN", "test.org")
	defer os.Unsetenv("CB365_INTERNAL_DOMAIN")

	got := getInternalDomain()
	if got != "test.org" {
		t.Errorf("getInternalDomain() = %q, want %q", got, "test.org")
	}
}


func TestContactsCreateRequiresName(t *testing.T) {
	cmd := contactsCreateCmd
	if cmd.Flags().Lookup("given-name") == nil {
		t.Fatal("contacts create missing --given-name flag")
	}
	if cmd.Flags().Lookup("surname") == nil {
		t.Fatal("contacts create missing --surname flag")
	}
}

func TestContactsUpdateRequiresID(t *testing.T) {
	cmd := contactsUpdateCmd
	if cmd.Flags().Lookup("id") == nil {
		t.Fatal("contacts update missing --id flag")
	}
}

func TestContactsFullCommandStructure(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range contactsCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"list", "get", "search", "create", "update"} {
		if !found[expected] {
			t.Errorf("contacts missing subcommand %q", expected)
		}
	}
}
