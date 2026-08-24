package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var (
	mailMoveID          string
	mailMoveDestination string
	mailMoveConfirm     bool
)

var mailMoveCmd = &cobra.Command{
	Use:   "move",
	Short: "Move a message to another mail folder",
	Long: "Move one message to a destination folder by ID or well-known folder name. " +
		"The operation is recoverable when the destination is deleteditems. " +
		"It never permanently deletes a message.",
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(mailMoveID)
		destinationID := strings.TrimSpace(mailMoveDestination)

		if messageID == "" {
			return fmt.Errorf("--id is required")
		}
		if destinationID == "" {
			return fmt.Errorf("--destination is required")
		}

		preview := map[string]interface{}{
			"action":      "move",
			"message_id":  messageID,
			"destination": destinationID,
			"dry_run":     flagDryRun,
		}
		if flagDryRun {
			return output.JSON(preview)
		}
		if !mailMoveConfirm {
			return fmt.Errorf("--confirm is required to move mail")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		body := users.NewItemMessagesItemMovePostRequestBody()
		body.SetDestinationId(&destinationID)

		moved, err := client.Me().Messages().ByMessageId(messageID).Move().Post(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("moving message: %w", err)
		}

		result := map[string]interface{}{
			"action":            "moved",
			"source_message_id": messageID,
			"destination":       destinationID,
			"message_id":        deref(moved.GetId()),
		}
		switch output.Resolve(flagJSON, flagPlain) {
		case output.FormatJSON:
			return output.JSON(result)
		case output.FormatPlain:
			output.Plain([][]string{{"moved", messageID, destinationID, deref(moved.GetId())}})
		default:
			output.Info(fmt.Sprintf("Moved message %s to %s", messageID, destinationID))
		}
		return nil
	},
}

func init() {
	mailMoveCmd.Flags().StringVar(&mailMoveID, "id", "", "Message ID")
	mailMoveCmd.Flags().StringVar(&mailMoveDestination, "destination", "", "Destination folder ID or well-known folder name")
	mailMoveCmd.Flags().BoolVar(&mailMoveConfirm, "confirm", false, "Confirm the move")
	mailCmd.AddCommand(mailMoveCmd)
}
