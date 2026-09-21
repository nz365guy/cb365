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
	mailReplyTo        string
	mailReplyCC        string
	mailReplyBody      string
	mailReplyMessageID string
	mailReplyAll       bool
	mailReplyConfirm   bool
)

var mailReplyCmd = &cobra.Command{
	Use:   "reply",
	Short: "Send a reply in an existing mail thread",
	Long: "Create a reply draft from an existing message, optionally add explicit " +
		"recipients, then send that draft. The original message ID is required so " +
		"the provider preserves the conversation thread.",
	RunE: func(cmd *cobra.Command, args []string) error {
		replyToID := strings.TrimSpace(mailReplyMessageID)
		body := strings.TrimSpace(mailReplyBody)
		if replyToID == "" {
			return fmt.Errorf("--reply-to-id is required")
		}
		if body == "" {
			return fmt.Errorf("--body is required")
		}
		if mailReplyAll && strings.TrimSpace(mailReplyTo) != "" {
			return fmt.Errorf("--to cannot be combined with --reply-all")
		}
		if countRecipients(mailReplyTo, mailReplyCC) > 10 {
			return fmt.Errorf("replying to more than 10 explicit recipients is not supported")
		}

		preview := map[string]interface{}{
			"action":      "reply",
			"reply_to_id": replyToID,
			"to":          mailReplyTo,
			"cc":          mailReplyCC,
			"dry_run":     flagDryRun,
		}
		if flagDryRun {
			return output.JSON(preview)
		}
		if !mailReplyConfirm {
			return fmt.Errorf("--confirm is required to send a reply")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var draft models.Messageable
		switch {
		case mailReplyAll:
			request := users.NewItemMessagesItemCreateReplyAllPostRequestBody()
			request.SetComment(ptr(body))
			draft, err = client.Me().Messages().ByMessageId(replyToID).CreateReplyAll().Post(ctx, request, nil)
		default:
			request := users.NewItemMessagesItemCreateReplyPostRequestBody()
			request.SetComment(ptr(body))
			draft, err = client.Me().Messages().ByMessageId(replyToID).CreateReply().Post(ctx, request, nil)
		}
		if err != nil {
			return fmt.Errorf("creating reply draft: %w", err)
		}
		draftID := deref(draft.GetId())
		if draftID == "" {
			return fmt.Errorf("creating reply draft returned no message id")
		}

		// CreateReply targets the original sender. Extra recipients are explicit
		// additions and are patched before the send operation.
		if !mailReplyAll && (strings.TrimSpace(mailReplyTo) != "" || strings.TrimSpace(mailReplyCC) != "") {
			patch := models.NewMessage()
			if to := parseRecipients(mailReplyTo); len(to) > 0 {
				patch.SetToRecipients(append(draft.GetToRecipients(), to...))
			}
			if cc := parseRecipients(mailReplyCC); len(cc) > 0 {
				patch.SetCcRecipients(append(draft.GetCcRecipients(), cc...))
			}
			if draft, err = client.Me().Messages().ByMessageId(draftID).Patch(ctx, patch, nil); err != nil {
				return fmt.Errorf("adding recipients to reply draft: %w", err)
			}
		}

		if err := client.Me().Messages().ByMessageId(draftID).Send().Post(ctx, nil); err != nil {
			return fmt.Errorf("sending reply: %w", err)
		}

		result := map[string]interface{}{
			"action":          "reply",
			"status":          "sent",
			"message_id":      draftID,
			"reply_to_id":     replyToID,
			"conversation_id": deref(draft.GetConversationId()),
		}
		if output.Resolve(flagJSON, flagPlain) == output.FormatJSON {
			return output.JSON(result)
		}
		output.Success(fmt.Sprintf("Sent reply in thread %s", replyToID))
		return nil
	},
}

func init() {
	mailReplyCmd.Flags().StringVar(&mailReplyTo, "to", "", "Additional recipient email (comma-separated)")
	mailReplyCmd.Flags().StringVar(&mailReplyCC, "cc", "", "Additional CC recipient email (comma-separated)")
	mailReplyCmd.Flags().StringVar(&mailReplyBody, "body", "", "Reply body (plain text)")
	mailReplyCmd.Flags().StringVar(&mailReplyMessageID, "reply-to-id", "", "Original message ID to reply to")
	mailReplyCmd.Flags().BoolVar(&mailReplyAll, "reply-all", false, "Reply to all original recipients")
	mailReplyCmd.Flags().BoolVar(&mailReplyConfirm, "confirm", false, "Confirm sending the reply")
	mailCmd.AddCommand(mailReplyCmd)
}
