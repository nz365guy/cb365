package main

import (
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func preserveMailListFolderGlobals(t *testing.T) {
	t.Helper()
	oldFolder, oldMax := mailListFolderID, mailListFolderMax
	oldFilter, oldOrder := mailListFolderFilter, mailListFolderOrder
	t.Cleanup(func() {
		mailListFolderID, mailListFolderMax = oldFolder, oldMax
		mailListFolderFilter, mailListFolderOrder = oldFilter, oldOrder
	})
}

func TestMailListFolderQuerySupportsMatchingFilterAndOrder(t *testing.T) {
	preserveMailListFolderGlobals(t)
	mailListFolderMax = 1000
	mailListFolderFilter = "lastModifiedDateTime lt 2026-08-18T00:00:00Z"
	mailListFolderOrder = "lastModifiedDateTime asc"

	parameters := newMailListFolderQueryParameters()
	if parameters.Filter == nil || *parameters.Filter != mailListFolderFilter {
		t.Fatalf("expected last-modified filter, got %v", parameters.Filter)
	}
	if len(parameters.Orderby) != 1 || parameters.Orderby[0] != mailListFolderOrder {
		t.Fatalf("expected matching last-modified order, got %v", parameters.Orderby)
	}
}

func TestMailListFolderQueryCanDisableOrdering(t *testing.T) {
	preserveMailListFolderGlobals(t)
	mailListFolderOrder = " "

	parameters := newMailListFolderQueryParameters()
	if len(parameters.Orderby) != 0 {
		t.Fatalf("expected no ordering, got %v", parameters.Orderby)
	}
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
