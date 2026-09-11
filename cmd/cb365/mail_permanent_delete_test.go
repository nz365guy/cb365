package main

import "testing"

func preserveMailPermanentDeleteGlobals(t *testing.T) {
	t.Helper()
	oldID, oldConfirm, oldDryRun := mailPermanentDeleteID, mailPermanentDeleteConfirm, flagDryRun
	t.Cleanup(func() {
		mailPermanentDeleteID, mailPermanentDeleteConfirm, flagDryRun = oldID, oldConfirm, oldDryRun
	})
}

func TestMailPermanentDeleteRequiresMessageIDBeforeAuthentication(t *testing.T) {
	preserveMailPermanentDeleteGlobals(t)
	mailPermanentDeleteID = " "
	mailPermanentDeleteConfirm = true
	flagDryRun = false

	err := mailPermanentDeleteCmd.RunE(mailPermanentDeleteCmd, nil)
	if err == nil || err.Error() != "--id is required" {
		t.Fatalf("expected missing id guard, got %v", err)
	}
}

func TestMailPermanentDeleteRequiresConfirmBeforeAuthentication(t *testing.T) {
	preserveMailPermanentDeleteGlobals(t)
	mailPermanentDeleteID = "message-id"
	mailPermanentDeleteConfirm = false
	flagDryRun = false

	err := mailPermanentDeleteCmd.RunE(mailPermanentDeleteCmd, nil)
	if err == nil || err.Error() != "--confirm is required to permanently delete mail" {
		t.Fatalf("expected confirmation guard, got %v", err)
	}
}

func TestMailPermanentDeleteDryRunDoesNotAuthenticate(t *testing.T) {
	preserveMailPermanentDeleteGlobals(t)
	mailPermanentDeleteID = "message-id"
	mailPermanentDeleteConfirm = false
	flagDryRun = true

	if err := mailPermanentDeleteCmd.RunE(mailPermanentDeleteCmd, nil); err != nil {
		t.Fatalf("dry run should stop before authentication: %v", err)
	}
}
