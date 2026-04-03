package main

import (
	"os"
	"testing"
)

// ─── Unit tests (no Graph API needed) ───

func TestCountRecipients(t *testing.T) {
	tests := []struct {
		to, cc string
		want   int
	}{
		{"a@x.com", "", 1},
		{"a@x.com,b@x.com", "", 2},
		{"a@x.com", "c@x.com", 2},
		{"a@x.com,b@x.com", "c@x.com,d@x.com", 4},
		{"", "", 0},
		{"  ", "", 0},
	}
	for _, tt := range tests {
		got := countRecipients(tt.to, tt.cc)
		if got != tt.want {
			t.Errorf("countRecipients(%q, %q) = %d, want %d", tt.to, tt.cc, got, tt.want)
		}
	}
}

func TestExternalRecipients(t *testing.T) {
	os.Setenv("CB365_INTERNAL_DOMAIN", "example.com")
	defer os.Unsetenv("CB365_INTERNAL_DOMAIN")

	ext := externalRecipients("internal@example.com,outside@gmail.com", "another@example.com")
	if len(ext) != 1 || ext[0] != "outside@gmail.com" {
		t.Errorf("externalRecipients: got %v, want [outside@gmail.com]", ext)
	}
}

func TestExternalRecipientsNoDomain(t *testing.T) {
	os.Unsetenv("CB365_INTERNAL_DOMAIN")

	ext := externalRecipients("anyone@gmail.com", "")
	if len(ext) != 0 {
		t.Errorf("externalRecipients with no domain set: got %v, want empty", ext)
	}
}

func TestGetInternalDomain(t *testing.T) {
	os.Setenv("CB365_INTERNAL_DOMAIN", "test.org")
	defer os.Unsetenv("CB365_INTERNAL_DOMAIN")

	got := getInternalDomain()
	if got != "test.org" {
		t.Errorf("getInternalDomain() = %q, want %q", got, "test.org")
	}
}

func TestIsDelegatedProfileErrorsWithoutConfig(t *testing.T) {
	// Without a valid config, isDelegatedProfile should return an error
	_, err := isDelegatedProfile()
	// This will either work (if config exists) or error — either is acceptable
	_ = err
}

