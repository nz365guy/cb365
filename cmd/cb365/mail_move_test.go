package main

import "testing"

func preserveMailMoveGlobals(t *testing.T) {
	t.Helper()
	oldID, oldDestination, oldConfirm, oldDryRun := mailMoveID, mailMoveDestination, mailMoveConfirm, flagDryRun
	t.Cleanup(func() {
		mailMoveID, mailMoveDestination, mailMoveConfirm, flagDryRun = oldID, oldDestination, oldConfirm, oldDryRun
	})
}

func TestMailMoveRequiresMessageIDBeforeAuthentication(t *testing.T) {
	preserveMailMoveGlobals(t)
	mailMoveID = ""
	mailMoveDestination = "deleteditems"
	mailMoveConfirm = true
	flagDryRun = false

	err := mailMoveCmd.RunE(mailMoveCmd, nil)
	if err == nil || err.Error() != "--id is required" {
		t.Fatalf("expected missing id guard, got %v", err)
	}
}

func TestMailMoveRequiresDestinationBeforeAuthentication(t *testing.T) {
	preserveMailMoveGlobals(t)
	mailMoveID = "message-id"
	mailMoveDestination = " "
	mailMoveConfirm = true
	flagDryRun = false

	err := mailMoveCmd.RunE(mailMoveCmd, nil)
	if err == nil || err.Error() != "--destination is required" {
		t.Fatalf("expected missing destination guard, got %v", err)
	}
}

func TestMailMoveRequiresConfirmBeforeAuthentication(t *testing.T) {
	preserveMailMoveGlobals(t)
	mailMoveID = "message-id"
	mailMoveDestination = "deleteditems"
	mailMoveConfirm = false
	flagDryRun = false

	err := mailMoveCmd.RunE(mailMoveCmd, nil)
	if err == nil || err.Error() != "--confirm is required to move mail" {
		t.Fatalf("expected confirmation guard, got %v", err)
	}
}

func TestMailMoveDryRunDoesNotAuthenticate(t *testing.T) {
	preserveMailMoveGlobals(t)
	mailMoveID = "message-id"
	mailMoveDestination = "deleteditems"
	mailMoveConfirm = false
	flagDryRun = true

	if err := mailMoveCmd.RunE(mailMoveCmd, nil); err != nil {
		t.Fatalf("dry run should stop before authentication: %v", err)
	}
}
