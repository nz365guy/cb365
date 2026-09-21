package main

import (
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func preserveMailReplyGlobals(t *testing.T) {
	t.Helper()
	oldTo, oldCC, oldBody, oldID := mailReplyTo, mailReplyCC, mailReplyBody, mailReplyMessageID
	oldAll, oldConfirm, oldDryRun := mailReplyAll, mailReplyConfirm, flagDryRun
	t.Cleanup(func() {
		mailReplyTo, mailReplyCC, mailReplyBody, mailReplyMessageID = oldTo, oldCC, oldBody, oldID
		mailReplyAll, mailReplyConfirm, flagDryRun = oldAll, oldConfirm, oldDryRun
	})
	mailReplyTo, mailReplyCC, mailReplyBody, mailReplyMessageID = "", "", "", ""
	mailReplyAll, mailReplyConfirm, flagDryRun = false, false, false
}

func TestMailReplyRequiresOriginalMessageBeforeAuthentication(t *testing.T) {
	preserveMailReplyGlobals(t)
	mailReplyBody = "body"
	mailReplyConfirm = true
	if err := mailReplyCmd.RunE(mailReplyCmd, nil); err == nil || err.Error() != "--reply-to-id is required" {
		t.Fatalf("expected original-message guard, got %v", err)
	}
}

func TestMailReplyRequiresConfirmationBeforeAuthentication(t *testing.T) {
	preserveMailReplyGlobals(t)
	mailReplyMessageID = "message-id"
	mailReplyBody = "body"
	if err := mailReplyCmd.RunE(mailReplyCmd, nil); err == nil || err.Error() != "--confirm is required to send a reply" {
		t.Fatalf("expected confirmation guard, got %v", err)
	}
}

func TestMailReplyAllRejectsExplicitRecipients(t *testing.T) {
	preserveMailReplyGlobals(t)
	mailReplyMessageID = "message-id"
	mailReplyBody = "body"
	mailReplyAll = true
	mailReplyTo = "extra@example.com"
	mailReplyConfirm = true
	if err := mailReplyCmd.RunE(mailReplyCmd, nil); err == nil || err.Error() != "--to cannot be combined with --reply-all" {
		t.Fatalf("expected reply-all recipient guard, got %v", err)
	}
}

func TestMailReplyDryRunDoesNotAuthenticate(t *testing.T) {
	preserveMailReplyGlobals(t)
	mailReplyMessageID = "message-id"
	mailReplyBody = "body"
	flagDryRun = true
	if err := mailReplyCmd.RunE(mailReplyCmd, nil); err != nil {
		t.Fatalf("dry run should stop before authentication: %v", err)
	}
}

func TestMergeRecipientsDoesNotDuplicateProviderDerivedReplyTarget(t *testing.T) {
	existing := []models.Recipientable{makeRecipient("Sender@Example.com")}
	merged := mergeRecipients(existing, []models.Recipientable{
		makeRecipient("sender@example.com"),
		makeRecipient("copy@example.com"),
	})
	if len(merged) != 2 {
		t.Fatalf("expected provider recipient plus one explicit recipient, got %d", len(merged))
	}
}
