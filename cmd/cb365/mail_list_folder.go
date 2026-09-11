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
	mailListFolderID     string
	mailListFolderMax    int32
	mailListFolderFilter string
	mailListFolderOrder  string
)

func newMailListFolderQueryParameters() *users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters {
	parameters := &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
		Top:    &mailListFolderMax,
		Select: []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "lastModifiedDateTime", "isRead", "hasAttachments", "importance", "bodyPreview"},
	}
	if order := strings.TrimSpace(mailListFolderOrder); order != "" {
		parameters.Orderby = []string{order}
	}
	if filter := strings.TrimSpace(mailListFolderFilter); filter != "" {
		parameters.Filter = &filter
	}
	return parameters
}

var mailListFolderCmd = &cobra.Command{
	Use:   "list-folder",
	Short: "List messages in a specific mail folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		folderID := strings.TrimSpace(mailListFolderID)
		if folderID == "" {
			return fmt.Errorf("--folder is required")
		}
		if mailListFolderMax < 1 || mailListFolderMax > 1000 {
			return fmt.Errorf("--max must be between 1 and 1000")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reqConfig := &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
			QueryParameters: newMailListFolderQueryParameters(),
		}

		result, err := client.Me().MailFolders().ByMailFolderId(folderID).Messages().Get(ctx, reqConfig)
		if err != nil {
			return fmt.Errorf("fetching messages from folder: %w", err)
		}

		messages := result.GetValue()
		switch output.Resolve(flagJSON, flagPlain) {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(messages))
			for _, msg := range messages {
				items = append(items, formatMessageJSON(msg))
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(messages))
			for _, msg := range messages {
				from := ""
				if msg.GetFrom() != nil {
					from = recipientString(msg.GetFrom())
				}
				rows = append(rows, []string{deref(msg.GetId()), from, deref(msg.GetSubject())})
			}
			output.Plain(rows)
		default:
			rows := make([][]string, 0, len(messages))
			for _, msg := range messages {
				from := ""
				if msg.GetFrom() != nil {
					from = recipientString(msg.GetFrom())
				}
				rows = append(rows, []string{from, deref(msg.GetSubject()), deref(msg.GetId())})
			}
			output.Table([]string{"FROM", "SUBJECT", "ID"}, rows)
		}
		return nil
	},
}

func init() {
	mailListFolderCmd.Flags().StringVar(&mailListFolderID, "folder", "", "Folder ID or well-known folder name")
	mailListFolderCmd.Flags().Int32Var(&mailListFolderMax, "max", 25, "Maximum messages to return")
	mailListFolderCmd.Flags().StringVar(&mailListFolderFilter, "filter", "", "OData filter expression")
	mailListFolderCmd.Flags().StringVar(&mailListFolderOrder, "order-by", "receivedDateTime desc", "OData order-by expression (empty disables ordering)")
	mailCmd.AddCommand(mailListFolderCmd)
}
