package main

import "testing"

func preserveMailDraftGlobals(t *testing.T) {
	t.Helper()
	oldTo, oldCC, oldSubject, oldBody := mailDraftTo, mailDraftCC, mailDraftSubject, mailDraftBody
	oldReplyTo, oldReplyAll, oldConfirm, oldDryRun := mailDraftReplyToID, mailDraftReplyAll, mailDraftConfirm, flagDryRun
	t.Cleanup(func() {
		mailDraftTo, mailDraftCC, mailDraftSubject, mailDraftBody = oldTo, oldCC, oldSubject, oldBody
		mailDraftReplyToID, mailDraftReplyAll, mailDraftConfirm, flagDryRun = oldReplyTo, oldReplyAll, oldConfirm, oldDryRun
	})
	mailDraftTo, mailDraftCC, mailDraftSubject, mailDraftBody = "", "", "", ""
	mailDraftReplyToID, mailDraftReplyAll, mailDraftConfirm, flagDryRun = "", false, false, false
}

func TestMailDraftRequiresBodyBeforeAuthentication(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftTo = "recipient@example.com"
	mailDraftSubject = "subject"
	mailDraftConfirm = true

	err := mailDraftCmd.RunE(mailDraftCmd, nil)
	if err == nil || err.Error() != "--body is required" {
		t.Fatalf("expected missing body guard, got %v", err)
	}
}

func TestMailDraftNewRequiresToAndSubjectBeforeAuthentication(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftBody = "body"
	mailDraftConfirm = true

	err := mailDraftCmd.RunE(mailDraftCmd, nil)
	if err == nil || err.Error() != "--to and --subject are required for a new draft (or use --reply-to-id)" {
		t.Fatalf("expected new-draft field guard, got %v", err)
	}
}

func TestMailDraftReplyAllRejectsExtraRecipients(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftBody = "body"
	mailDraftReplyToID = "message-id"
	mailDraftReplyAll = true
	mailDraftTo = "extra@example.com"
	mailDraftConfirm = true

	err := mailDraftCmd.RunE(mailDraftCmd, nil)
	if err == nil || err.Error() != "--to cannot be combined with --reply-all" {
		t.Fatalf("expected reply-all recipient guard, got %v", err)
	}
}

func TestMailDraftRequiresConfirmBeforeAuthentication(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftBody = "body"
	mailDraftReplyToID = "message-id"
	mailDraftConfirm = false

	err := mailDraftCmd.RunE(mailDraftCmd, nil)
	if err == nil || err.Error() != "--confirm is required to create a draft" {
		t.Fatalf("expected confirmation guard, got %v", err)
	}
}

func TestMailDraftBlastRadiusGuard(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftBody = "body"
	mailDraftSubject = "subject"
	mailDraftTo = "a@example.com,b@example.com,c@example.com,d@example.com,e@example.com,f@example.com"
	mailDraftCC = "g@example.com,h@example.com,i@example.com,j@example.com,k@example.com"
	mailDraftConfirm = true

	err := mailDraftCmd.RunE(mailDraftCmd, nil)
	if err == nil || err.Error() != "drafting to 11 recipients is not supported (blast radius guard)" {
		t.Fatalf("expected blast radius guard, got %v", err)
	}
}

func TestMailDraftDryRunDoesNotAuthenticate(t *testing.T) {
	preserveMailDraftGlobals(t)
	mailDraftBody = "body"
	mailDraftReplyToID = "message-id"
	mailDraftConfirm = false
	flagDryRun = true

	if err := mailDraftCmd.RunE(mailDraftCmd, nil); err != nil {
		t.Fatalf("dry run should stop before authentication: %v", err)
	}
}

func TestParseRecipientsTrimsAndSkipsEmpty(t *testing.T) {
	recipients := parseRecipients(" a@example.com , ,b@example.com,")
	if len(recipients) != 2 {
		t.Fatalf("expected 2 recipients, got %d", len(recipients))
	}
}
