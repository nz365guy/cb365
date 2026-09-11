package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var (
	mailPermanentDeleteID      string
	mailPermanentDeleteConfirm bool
)

var mailPermanentDeleteCmd = &cobra.Command{
	Use:   "permanent-delete",
	Short: "Permanently delete one mail message",
	Long: "Permanently delete one message by ID using Microsoft Graph permanentDelete. " +
		"This is irreversible from Outlook and requires --confirm. Use --dry-run to preview without authentication.",
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(mailPermanentDeleteID)
		if messageID == "" {
			return fmt.Errorf("--id is required")
		}

		preview := map[string]interface{}{
			"action":     "permanent-delete",
			"message_id": messageID,
			"dry_run":    flagDryRun,
		}
		if flagDryRun {
			return output.JSON(preview)
		}
		if !mailPermanentDeleteConfirm {
			return fmt.Errorf("--confirm is required to permanently delete mail")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Me().Messages().ByMessageId(messageID).PermanentDelete().Post(ctx, nil); err != nil {
			return fmt.Errorf("permanently deleting message: %w", err)
		}

		result := map[string]interface{}{
			"action":     "permanently-deleted",
			"message_id": messageID,
		}
		switch output.Resolve(flagJSON, flagPlain) {
		case output.FormatJSON:
			return output.JSON(result)
		case output.FormatPlain:
			output.Plain([][]string{{"permanently-deleted", messageID}})
		default:
			output.Info(fmt.Sprintf("Permanently deleted message %s", messageID))
		}
		return nil
	},
}

func init() {
	mailPermanentDeleteCmd.Flags().StringVar(&mailPermanentDeleteID, "id", "", "Message ID")
	mailPermanentDeleteCmd.Flags().BoolVar(&mailPermanentDeleteConfirm, "confirm", false, "Confirm permanent deletion")
	mailCmd.AddCommand(mailPermanentDeleteCmd)
}
