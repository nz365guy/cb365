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

