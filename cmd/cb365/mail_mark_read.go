package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var (
	mailMarkReadID      string
	mailMarkReadConfirm bool
)

var mailMarkReadCmd = &cobra.Command{
	Use:   "mark-read",
	Short: "Mark a mail message as read",
	Long: "Mark one message as read by ID. The operation only updates the read flag; " +
		"it does not move, delete, send, or otherwise change the message.",
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(mailMarkReadID)
		if messageID == "" {
			return fmt.Errorf("--id is required")
		}

		preview := map[string]interface{}{
			"action":     "mark_read",
			"message_id": messageID,
			"is_read":    true,
			"dry_run":    flagDryRun,
		}
		if flagDryRun {
			return output.JSON(preview)
		}
		if !mailMarkReadConfirm {
			return fmt.Errorf("--confirm is required to mark mail as read")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		patch := models.NewMessage()
		isRead := true
		patch.SetIsRead(&isRead)
		updated, err := client.Me().Messages().ByMessageId(messageID).Patch(ctx, patch, nil)
		if err != nil {
			return fmt.Errorf("marking message as read: %w", err)
		}

		result := map[string]interface{}{
			"action":     "marked_read",
			"message_id": messageID,
			"is_read":    updated.GetIsRead() != nil && *updated.GetIsRead(),
		}
		switch output.Resolve(flagJSON, flagPlain) {
		case output.FormatJSON:
			return output.JSON(result)
		case output.FormatPlain:
			output.Plain([][]string{{"marked_read", messageID, "true"}})
		default:
			output.Info(fmt.Sprintf("Marked message %s as read", messageID))
		}
		return nil
	},
}

func init() {
	mailMarkReadCmd.Flags().StringVar(&mailMarkReadID, "id", "", "Message ID")
	mailMarkReadCmd.Flags().BoolVar(&mailMarkReadConfirm, "confirm", false, "Confirm the read-state update")
	mailCmd.AddCommand(mailMarkReadCmd)
}
