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
	if len(ext) != 1 || ext[0] != "anyone@gmail.com" {
		t.Errorf("externalRecipients with no domain set: got %v, want conservative classification", ext)
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

func TestMailSendRequiresConfirmBeforeAuthentication(t *testing.T) {
	oldTo, oldSubject, oldBody, oldConfirm := mailSendTo, mailSendSubject, mailSendBody, mailSendConfirm
	defer func() {
		mailSendTo, mailSendSubject, mailSendBody, mailSendConfirm = oldTo, oldSubject, oldBody, oldConfirm
	}()

	mailSendTo = "recipient@example.com"
	mailSendSubject = "test"
	mailSendBody = "test"
	mailSendConfirm = false

	err := mailSendCmd.RunE(mailSendCmd, nil)
	if err == nil || err.Error() != "--confirm is required to send mail (safety guard against accidental sends)" {
		t.Fatalf("expected universal confirmation guard, got %v", err)
	}
}
