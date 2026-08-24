package main

import "testing"

func preserveMailListFolderGlobals(t *testing.T) {
	t.Helper()
	oldFolder, oldMax := mailListFolderID, mailListFolderMax
	t.Cleanup(func() {
		mailListFolderID, mailListFolderMax = oldFolder, oldMax
	})
}

func TestMailListFolderRequiresFolderBeforeAuthentication(t *testing.T) {
	preserveMailListFolderGlobals(t)
	mailListFolderID = " "
	mailListFolderMax = 25

	err := mailListFolderCmd.RunE(mailListFolderCmd, nil)
	if err == nil || err.Error() != "--folder is required" {
		t.Fatalf("expected missing folder guard, got %v", err)
	}
}

func TestMailListFolderRejectsInvalidMaximumBeforeAuthentication(t *testing.T) {
	preserveMailListFolderGlobals(t)
	mailListFolderID = "inbox"
	mailListFolderMax = 0

	err := mailListFolderCmd.RunE(mailListFolderCmd, nil)
	if err == nil || err.Error() != "--max must be between 1 and 1000" {
		t.Fatalf("expected maximum guard, got %v", err)
	}
}
