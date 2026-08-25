package main

import "testing"

func preserveMailMarkReadGlobals(t *testing.T) {
	t.Helper()
	oldID, oldConfirm, oldDryRun := mailMarkReadID, mailMarkReadConfirm, flagDryRun
	t.Cleanup(func() {
		mailMarkReadID, mailMarkReadConfirm, flagDryRun = oldID, oldConfirm, oldDryRun
	})
}

func TestMailMarkReadRequiresMessageIDBeforeAuthentication(t *testing.T) {
	preserveMailMarkReadGlobals(t)
	mailMarkReadID = ""
	mailMarkReadConfirm = true
	flagDryRun = false

	err := mailMarkReadCmd.RunE(mailMarkReadCmd, nil)
	if err == nil || err.Error() != "--id is required" {
		t.Fatalf("expected missing id guard, got %v", err)
	}
}

func TestMailMarkReadRequiresConfirmBeforeAuthentication(t *testing.T) {
	preserveMailMarkReadGlobals(t)
	mailMarkReadID = "message-id"
	mailMarkReadConfirm = false
	flagDryRun = false

	err := mailMarkReadCmd.RunE(mailMarkReadCmd, nil)
	if err == nil || err.Error() != "--confirm is required to mark mail as read" {
		t.Fatalf("expected confirmation guard, got %v", err)
	}
}

func TestMailMarkReadDryRunDoesNotAuthenticate(t *testing.T) {
	preserveMailMarkReadGlobals(t)
	mailMarkReadID = "message-id"
	mailMarkReadConfirm = false
	flagDryRun = true

	if err := mailMarkReadCmd.RunE(mailMarkReadCmd, nil); err != nil {
		t.Fatalf("dry run should stop before authentication: %v", err)
	}
}
