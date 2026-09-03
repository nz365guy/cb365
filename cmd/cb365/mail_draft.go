package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var (
	mailDraftTo        string
	mailDraftCC        string
	mailDraftSubject   string
	mailDraftBody      string
	mailDraftReplyToID string
	mailDraftReplyAll  bool
	mailDraftConfirm   bool
)

var mailDraftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Create a draft message or a draft reply (never sends)",
	Long: "Create a draft in the Drafts folder, either a new message (--to, --subject, --body) " +
		"or a reply to an existing message (--reply-to-id with --body, optionally --reply-all). " +
		"The draft is saved for a person to review and send from their mail client. " +
		"This command has no send path.",
	RunE: func(cmd *cobra.Command, args []string) error {
		replyToID := strings.TrimSpace(mailDraftReplyToID)
		body := strings.TrimSpace(mailDraftBody)

		if body == "" {
			return fmt.Errorf("--body is required")
		}
		if replyToID == "" {
			if strings.TrimSpace(mailDraftTo) == "" || strings.TrimSpace(mailDraftSubject) == "" {
				return fmt.Errorf("--to and --subject are required for a new draft (or use --reply-to-id)")
			}
		} else if mailDraftReplyAll && strings.TrimSpace(mailDraftTo) != "" {
			return fmt.Errorf("--to cannot be combined with --reply-all")
		}

		// Safety: >10 recipients requires an explicit decision; drafts inherit the send guard.
		if totalRecipients := countRecipients(mailDraftTo, mailDraftCC); totalRecipients > 10 {
			return fmt.Errorf("drafting to %d recipients is not supported (blast radius guard)", totalRecipients)
		}

		action := "draft_new"
		if replyToID != "" {
			action = "draft_reply"
			if mailDraftReplyAll {
				action = "draft_reply_all"
			}
		}
		preview := map[string]interface{}{
			"action":      action,
			"reply_to_id": replyToID,
			"to":          mailDraftTo,
			"cc":          mailDraftCC,
			"subject":     mailDraftSubject,
			"dry_run":     flagDryRun,
		}
		if flagDryRun {
			return output.JSON(preview)
		}
		if !mailDraftConfirm {
			return fmt.Errorf("--confirm is required to create a draft")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var draft models.Messageable
		switch {
		case replyToID != "" && mailDraftReplyAll:
			request := users.NewItemMessagesItemCreateReplyAllPostRequestBody()
			request.SetComment(ptr(body))
			draft, err = client.Me().Messages().ByMessageId(replyToID).CreateReplyAll().Post(ctx, request, nil)
		case replyToID != "":
			request := users.NewItemMessagesItemCreateReplyPostRequestBody()
			request.SetComment(ptr(body))
			draft, err = client.Me().Messages().ByMessageId(replyToID).CreateReply().Post(ctx, request, nil)
		default:
			msg := models.NewMessage()
			msg.SetSubject(ptr(strings.TrimSpace(mailDraftSubject)))
			content := models.NewItemBody()
			content.SetContent(ptr(body))
			contentType := models.TEXT_BODYTYPE
			content.SetContentType(&contentType)
			msg.SetBody(content)
			msg.SetToRecipients(parseRecipients(mailDraftTo))
			if cc := parseRecipients(mailDraftCC); len(cc) > 0 {
				msg.SetCcRecipients(cc)
			}
			draft, err = client.Me().Messages().Post(ctx, msg, nil)
		}
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		// A reply draft created from a comment keeps the original thread; a
		// caller who supplied --to on a plain reply wants extra recipients added.
		if replyToID != "" && !mailDraftReplyAll && strings.TrimSpace(mailDraftTo) != "" {
			patch := models.NewMessage()
			patch.SetToRecipients(append(draft.GetToRecipients(), parseRecipients(mailDraftTo)...))
			if cc := parseRecipients(mailDraftCC); len(cc) > 0 {
				patch.SetCcRecipients(append(draft.GetCcRecipients(), cc...))
			}
			if draft, err = client.Me().Messages().ByMessageId(deref(draft.GetId())).Patch(ctx, patch, nil); err != nil {
				return fmt.Errorf("adding recipients to draft: %w", err)
			}
		}

		result := map[string]interface{}{
			"action":     action,
			"status":     "drafted",
			"message_id": deref(draft.GetId()),
			"subject":    deref(draft.GetSubject()),
			"to":         recipientListString(draft.GetToRecipients()),
			"is_draft":   true,
			"web_link":   deref(draft.GetWebLink()),
		}
		if replyToID != "" {
			result["reply_to_id"] = replyToID
		}
		switch output.Resolve(flagJSON, flagPlain) {
		case output.FormatJSON:
			return output.JSON(result)
		case output.FormatPlain:
			output.Plain([][]string{{"drafted", deref(draft.GetId()), deref(draft.GetSubject())}})
		default:
			output.Success(fmt.Sprintf("Draft saved: %s — %s", deref(draft.GetId()), deref(draft.GetSubject())))
		}
		return nil
	},
}

// parseRecipients splits a comma-separated address list into Graph recipients.
func parseRecipients(list string) []models.Recipientable {
	recipients := make([]models.Recipientable, 0)
	for _, addr := range strings.Split(list, ",") {
		if trimmed := strings.TrimSpace(addr); trimmed != "" {
			recipients = append(recipients, makeRecipient(trimmed))
		}
	}
	return recipients
}

func init() {
	mailDraftCmd.Flags().StringVar(&mailDraftTo, "to", "", "Recipient email (comma-separated for multiple)")
	mailDraftCmd.Flags().StringVar(&mailDraftCC, "cc", "", "CC recipients (comma-separated)")
	mailDraftCmd.Flags().StringVar(&mailDraftSubject, "subject", "", "Draft subject (new drafts)")
	mailDraftCmd.Flags().StringVar(&mailDraftBody, "body", "", "Draft body (plain text)")
	mailDraftCmd.Flags().StringVar(&mailDraftReplyToID, "reply-to-id", "", "Message ID to reply to (creates a reply draft in the same thread)")
	mailDraftCmd.Flags().BoolVar(&mailDraftReplyAll, "reply-all", false, "Reply to all original recipients (with --reply-to-id)")
	mailDraftCmd.Flags().BoolVar(&mailDraftConfirm, "confirm", false, "Confirm creating the draft")
	mailCmd.AddCommand(mailDraftCmd)
}
