package main

import (
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

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

func TestFormatMessageJSONIncludesLastModifiedTime(t *testing.T) {
	message := models.NewMessage()
	modified := time.Date(2026, time.August, 25, 1, 2, 3, 0, time.UTC)
	message.SetLastModifiedDateTime(&modified)

	formatted := formatMessageJSON(message)
	if got := formatted["last_modified_at"]; got != "2026-08-25T01:02:03Z" {
		t.Fatalf("expected last_modified_at, got %v", got)
	}
}
