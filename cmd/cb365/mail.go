package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Mail helpers
// ──────────────────────────────────────────────

// recipientString extracts "Name <email>" or just "email" from a Recipient.
func recipientString(r models.Recipientable) string {
	if r == nil || r.GetEmailAddress() == nil {
		return ""
	}
	addr := deref(r.GetEmailAddress().GetAddress())
	name := deref(r.GetEmailAddress().GetName())
	if name != "" && name != addr {
		return fmt.Sprintf("%s <%s>", name, addr)
	}
	return addr
}

// recipientListString joins multiple recipients with ", ".
func recipientListString(recipients []models.Recipientable) string {
	parts := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if s := recipientString(r); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// makeRecipient creates a Recipient from an email address string.
func makeRecipient(email string) models.Recipientable {
	addr := models.NewEmailAddress()
	addr.SetAddress(ptr(strings.TrimSpace(email)))

	r := models.NewRecipient()
	r.SetEmailAddress(addr)
	return r
}

// isDelegatedProfile checks if the active profile uses delegated auth.
func isDelegatedProfile() (bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}
	profileName := flagProfile
	if profileName == "" {
		profileName = cfg.ActiveProfile
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return false, fmt.Errorf("profile %q not found", profileName)
	}
	return profile.AuthMode == config.AuthModeDelegated, nil
}

// formatMessageJSON builds a JSON-serialisable map from a Message.
func formatMessageJSON(msg models.Messageable) map[string]interface{} {
	item := map[string]interface{}{
		"id":      deref(msg.GetId()),
		"subject": deref(msg.GetSubject()),
	}

	if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil {
		item["from"] = map[string]string{
			"name":    deref(msg.GetFrom().GetEmailAddress().GetName()),
			"address": deref(msg.GetFrom().GetEmailAddress().GetAddress()),
		}
	}

	to := make([]map[string]string, 0)
	for _, r := range msg.GetToRecipients() {
		if r.GetEmailAddress() != nil {
			to = append(to, map[string]string{
				"name":    deref(r.GetEmailAddress().GetName()),
				"address": deref(r.GetEmailAddress().GetAddress()),
			})
		}
	}
	item["to"] = to

	if msg.GetReceivedDateTime() != nil {
		item["received_at"] = msg.GetReceivedDateTime().Format(time.RFC3339)
	}
	if msg.GetSentDateTime() != nil {
		item["sent_at"] = msg.GetSentDateTime().Format(time.RFC3339)
	}
	if msg.GetIsRead() != nil {
		item["is_read"] = *msg.GetIsRead()
	}
	if msg.GetHasAttachments() != nil {
		item["has_attachments"] = *msg.GetHasAttachments()
	}
	if msg.GetImportance() != nil {
		item["importance"] = msg.GetImportance().String()
	}
	item["body_preview"] = deref(msg.GetBodyPreview())

	return item
}

// ──────────────────────────────────────────────
//  Parent command
// ──────────────────────────────────────────────

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Outlook Mail — list, read, send, search",
}

// ══════════════════════════════════════════════
//  MAIL LIST
// ══════════════════════════════════════════════

var (
	mailListMax    int32
	mailListFilter string
)

var mailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in the inbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reqConfig := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
				Top:     &mailListMax,
				Orderby: []string{"receivedDateTime desc"},
				Select:  []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "isRead", "hasAttachments", "importance", "bodyPreview"},
			},
		}
		if mailListFilter != "" {
			reqConfig.QueryParameters.Filter = &mailListFilter
		}

		result, err := client.Me().Messages().Get(ctx, reqConfig)
		if err != nil {
			return fmt.Errorf("fetching messages: %w", err)
		}

		messages := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(messages))
			for _, msg := range messages {
				items = append(items, formatMessageJSON(msg))
			}
			return output.JSON(items)

		case output.FormatPlain:
			var rows [][]string
			for _, msg := range messages {
				received := ""
				if msg.GetReceivedDateTime() != nil {
					received = msg.GetReceivedDateTime().Format("2006-01-02 15:04")
				}
				from := ""
				if msg.GetFrom() != nil {
					from = recipientString(msg.GetFrom())
				}
				rows = append(rows, []string{
					deref(msg.GetId()),
					received,
					from,
					deref(msg.GetSubject()),
				})
			}
			output.Plain(rows)

		default:
			headers := []string{"DATE", "FROM", "SUBJECT", "READ", "ID"}
			var rows [][]string
			for _, msg := range messages {
				received := ""
				if msg.GetReceivedDateTime() != nil {
					received = msg.GetReceivedDateTime().Format("2 Jan 3:04pm")
				}
				from := ""
				if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil {
					from = deref(msg.GetFrom().GetEmailAddress().GetName())
					if from == "" {
						from = deref(msg.GetFrom().GetEmailAddress().GetAddress())
					}
				}
				read := "·"
				if msg.GetIsRead() != nil && *msg.GetIsRead() {
					read = "✓"
				}
				subject := deref(msg.GetSubject())
				if len(subject) > 60 {
					subject = subject[:57] + "..."
				}
				id := deref(msg.GetId())
				if len(id) > 20 {
					id = id[:17] + "..."
				}
				rows = append(rows, []string{received, from, subject, read, id})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  MAIL GET
// ══════════════════════════════════════════════

var mailGetID string

var mailGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single message by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		if mailGetID == "" {
			return fmt.Errorf("--id is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		msg, err := client.Me().Messages().ByMessageId(mailGetID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching message: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			item := formatMessageJSON(msg)
			// Include full body for get
			if msg.GetBody() != nil {
				item["body"] = deref(msg.GetBody().GetContent())
				item["body_type"] = msg.GetBody().GetContentType().String()
			}
			// Include CC and BCC
			cc := make([]map[string]string, 0)
			for _, r := range msg.GetCcRecipients() {
				if r.GetEmailAddress() != nil {
					cc = append(cc, map[string]string{
						"name":    deref(r.GetEmailAddress().GetName()),
						"address": deref(r.GetEmailAddress().GetAddress()),
					})
				}
			}
			item["cc"] = cc
			if msg.GetConversationId() != nil {
				item["conversation_id"] = deref(msg.GetConversationId())
			}
			if msg.GetWebLink() != nil {
				item["web_link"] = deref(msg.GetWebLink())
			}
			return output.JSON(item)

		default:
			fmt.Printf("Subject:  %s\n", deref(msg.GetSubject()))
			if msg.GetFrom() != nil {
				fmt.Printf("From:     %s\n", recipientString(msg.GetFrom()))
			}
			fmt.Printf("To:       %s\n", recipientListString(msg.GetToRecipients()))
			if len(msg.GetCcRecipients()) > 0 {
				fmt.Printf("CC:       %s\n", recipientListString(msg.GetCcRecipients()))
			}
			if msg.GetReceivedDateTime() != nil {
				fmt.Printf("Received: %s\n", msg.GetReceivedDateTime().Format("2 Jan 2006 3:04pm MST"))
			}
			read := "No"
			if msg.GetIsRead() != nil && *msg.GetIsRead() {
				read = "Yes"
			}
			fmt.Printf("Read:     %s\n", read)
			fmt.Printf("ID:       %s\n", deref(msg.GetId()))
			if msg.GetBody() != nil && deref(msg.GetBody().GetContent()) != "" {
				fmt.Println()
				fmt.Println("─── Body ───")
				fmt.Println(deref(msg.GetBody().GetContent()))
			}
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  MAIL SEND
// ══════════════════════════════════════════════

var (
	mailSendTo      string
	mailSendCC      string
	mailSendSubject string
	mailSendBody    string
	mailSendConfirm bool
)

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a mail message",
	RunE: func(cmd *cobra.Command, args []string) error {
		if mailSendTo == "" || mailSendSubject == "" || mailSendBody == "" {
			return fmt.Errorf("--to, --subject, and --body are required")
		}

		// Safety: delegated mode requires --confirm
		delegated, err := isDelegatedProfile()
		if err != nil {
			return err
		}
		if delegated && !mailSendConfirm {
			return fmt.Errorf("delegated mode requires --confirm to send mail (safety guard against accidental sends)")
		}

		format := output.Resolve(flagJSON, flagPlain)

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would send mail to %s — Subject: %s", mailSendTo, mailSendSubject))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true,
					"action":  "send_mail",
					"to":      mailSendTo,
					"subject": mailSendSubject,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build message
		msg := models.NewMessage()
		msg.SetSubject(ptr(mailSendSubject))

		body := models.NewItemBody()
		body.SetContent(ptr(mailSendBody))
		contentType := models.TEXT_BODYTYPE
		body.SetContentType(&contentType)
		msg.SetBody(body)

		// Parse recipients (comma-separated)
		toAddrs := strings.Split(mailSendTo, ",")
		toRecipients := make([]models.Recipientable, 0, len(toAddrs))
		for _, addr := range toAddrs {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				toRecipients = append(toRecipients, makeRecipient(trimmed))
			}
		}
		msg.SetToRecipients(toRecipients)

		// CC (optional)
		if mailSendCC != "" {
			ccAddrs := strings.Split(mailSendCC, ",")
			ccRecipients := make([]models.Recipientable, 0, len(ccAddrs))
			for _, addr := range ccAddrs {
				if trimmed := strings.TrimSpace(addr); trimmed != "" {
					ccRecipients = append(ccRecipients, makeRecipient(trimmed))
				}
			}
			msg.SetCcRecipients(ccRecipients)
		}

		// Build sendMail request body
		sendBody := users.NewItemSendMailPostRequestBody()
		sendBody.SetMessage(msg)
		saveToSent := true
		sendBody.SetSaveToSentItems(&saveToSent)

		if err := client.Me().SendMail().Post(ctx, sendBody, nil); err != nil {
			return fmt.Errorf("sending mail: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"status":  "sent",
				"to":      mailSendTo,
				"subject": mailSendSubject,
			})
		default:
			output.Success(fmt.Sprintf("Sent mail to %s — %s", mailSendTo, mailSendSubject))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  MAIL SEARCH
// ══════════════════════════════════════════════

var (
	mailSearchQuery string
	mailSearchMax   int32
)

var mailSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search messages by keyword",
	RunE: func(cmd *cobra.Command, args []string) error {
		if mailSearchQuery == "" {
			return fmt.Errorf("--query is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		searchStr := fmt.Sprintf("\"%s\"", mailSearchQuery)
		reqConfig := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
				Search:  &searchStr,
				Top:     &mailSearchMax,
				Select:  []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "isRead", "hasAttachments", "importance", "bodyPreview"},
			},
		}

		result, err := client.Me().Messages().Get(ctx, reqConfig)
		if err != nil {
			return fmt.Errorf("searching messages: %w", err)
		}

		messages := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(messages))
			for _, msg := range messages {
				items = append(items, formatMessageJSON(msg))
			}
			return output.JSON(map[string]interface{}{
				"query":   mailSearchQuery,
				"count":   len(items),
				"results": items,
			})

		case output.FormatPlain:
			var rows [][]string
			for _, msg := range messages {
				received := ""
				if msg.GetReceivedDateTime() != nil {
					received = msg.GetReceivedDateTime().Format("2006-01-02 15:04")
				}
				from := ""
				if msg.GetFrom() != nil {
					from = recipientString(msg.GetFrom())
				}
				rows = append(rows, []string{
					deref(msg.GetId()),
					received,
					from,
					deref(msg.GetSubject()),
				})
			}
			output.Plain(rows)

		default:
			output.Info(fmt.Sprintf("Search results for %q (%d found)", mailSearchQuery, len(messages)))
			headers := []string{"DATE", "FROM", "SUBJECT", "ID"}
			var rows [][]string
			for _, msg := range messages {
				received := ""
				if msg.GetReceivedDateTime() != nil {
					received = msg.GetReceivedDateTime().Format("2 Jan 3:04pm")
				}
				from := ""
				if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil {
					from = deref(msg.GetFrom().GetEmailAddress().GetName())
					if from == "" {
						from = deref(msg.GetFrom().GetEmailAddress().GetAddress())
					}
				}
				subject := deref(msg.GetSubject())
				if len(subject) > 60 {
					subject = subject[:57] + "..."
				}
				id := deref(msg.GetId())
				if len(id) > 20 {
					id = id[:17] + "..."
				}
				rows = append(rows, []string{received, from, subject, id})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  Wire up commands + flags
// ══════════════════════════════════════════════

func init() {
	// mail list
	mailListCmd.Flags().Int32Var(&mailListMax, "max", 25, "Maximum messages to return")
	mailListCmd.Flags().StringVar(&mailListFilter, "filter", "", "OData filter expression")

	// mail get
	mailGetCmd.Flags().StringVar(&mailGetID, "id", "", "Message ID")

	// mail send
	mailSendCmd.Flags().StringVar(&mailSendTo, "to", "", "Recipient email (comma-separated for multiple)")
	mailSendCmd.Flags().StringVar(&mailSendCC, "cc", "", "CC recipients (comma-separated)")
	mailSendCmd.Flags().StringVar(&mailSendSubject, "subject", "", "Email subject")
	mailSendCmd.Flags().StringVar(&mailSendBody, "body", "", "Email body (plain text)")
	mailSendCmd.Flags().BoolVar(&mailSendConfirm, "confirm", false, "Confirm send (required in delegated mode)")

	// mail search
	mailSearchCmd.Flags().StringVar(&mailSearchQuery, "query", "", "Search query")
	mailSearchCmd.Flags().Int32Var(&mailSearchMax, "max", 25, "Maximum results to return")

	// Wire
	mailCmd.AddCommand(mailListCmd)
	mailCmd.AddCommand(mailGetCmd)
	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailSearchCmd)
}

