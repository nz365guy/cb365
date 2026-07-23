package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	chatsPkg "github.com/microsoftgraph/msgraph-sdk-go/chats"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	teamsPkg "github.com/microsoftgraph/msgraph-sdk-go/teams"
	usersPkg "github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/auth"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Teams constants
// ──────────────────────────────────────────────

// teamsAuditFooter is appended to every agent-generated message.
// Ensures an observable audit trail differentiating autonomous
// communication from human interaction.
const teamsAuditFooter = "\n\n[Sent via cb365]"

// teamsAuditFooterHTML is the HTML equivalent of teamsAuditFooter, used when
// a message is sent with --html. Same visible text — plain newlines do not
// render as line breaks in HTML bodies, so <br> is required.
const teamsAuditFooterHTML = "<br><br>[Sent via cb365]"

// taggedTeamsBody appends the audit footer to body and returns the tagged
// content plus the Graph body type for the requested rendering mode.
func taggedTeamsBody(body string, html bool) (string, models.BodyType) {
	if html {
		return body + teamsAuditFooterHTML, models.HTML_BODYTYPE
	}
	return body + teamsAuditFooter, models.TEXT_BODYTYPE
}

// htmlBodyGuard rejects HTML bodies that could swallow the appended audit
// footer: an unterminated <!-- consumes everything after it when the message
// is parsed as HTML, hiding the attribution.
func htmlBodyGuard(body string) error {
	if strings.Contains(body, "<!--") {
		return fmt.Errorf("--html body must not contain HTML comments (<!--) — they can hide the audit footer")
	}
	return nil
}

// ──────────────────────────────────────────────
//  Teams helpers
// ──────────────────────────────────────────────

// resolveTeamID resolves a team display name or ID to a Graph team ID.
func resolveTeamID(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, nameOrID string) (string, string, error) {
	// If it looks like a GUID, use directly
	if len(nameOrID) == 36 && strings.Count(nameOrID, "-") == 4 {
		return nameOrID, nameOrID, nil
	}

	result, err := client.Me().JoinedTeams().Get(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("listing joined teams for name resolution: %w", err)
	}

	target := strings.ToLower(nameOrID)
	for _, team := range result.GetValue() {
		if strings.ToLower(deref(team.GetDisplayName())) == target {
			return deref(team.GetId()), deref(team.GetDisplayName()), nil
		}
	}

	return "", "", fmt.Errorf("team %q not found in your joined teams", nameOrID)
}

// resolveChannelID resolves a channel display name or ID to a Graph channel ID.
func resolveChannelID(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, teamID, nameOrID string) (string, string, error) {
	// If it looks like a long Graph ID, use directly
	if strings.Contains(nameOrID, ":") || (len(nameOrID) > 36 && !strings.Contains(nameOrID, " ")) {
		return nameOrID, nameOrID, nil
	}

	result, err := client.Teams().ByTeamId(teamID).Channels().Get(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("listing channels for name resolution: %w", err)
	}

	target := strings.ToLower(nameOrID)
	for _, ch := range result.GetValue() {
		if strings.ToLower(deref(ch.GetDisplayName())) == target {
			return deref(ch.GetId()), deref(ch.GetDisplayName()), nil
		}
	}

	return "", "", fmt.Errorf("channel %q not found in team %s", nameOrID, teamID)
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var teamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Microsoft Teams — channels and chat messaging",
}

var teamsChannelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "Manage Teams channels",
}

var teamsChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Manage Teams chats",
}

// ──────────────────────────────────────────────
//  teams channels list
// ──────────────────────────────────────────────

var teamsChannelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List channels in a team",
	RunE: func(cmd *cobra.Command, args []string) error {
		teamFlag, _ := cmd.Flags().GetString("team")
		if teamFlag == "" {
			return fmt.Errorf("--team is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		teamID, _, err := resolveTeamID(ctx, client, teamFlag)
		if err != nil {
			return err
		}

		result, err := client.Teams().ByTeamId(teamID).Channels().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing channels: %w", err)
		}

		channels := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(channels))
			for _, ch := range channels {
				item := map[string]interface{}{
					"id":          deref(ch.GetId()),
					"displayName": deref(ch.GetDisplayName()),
					"description": deref(ch.GetDescription()),
					"webUrl":      deref(ch.GetWebUrl()),
				}
				if ch.GetMembershipType() != nil {
					item["membershipType"] = ch.GetMembershipType().String()
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(channels))
			for _, ch := range channels {
				rows = append(rows, []string{deref(ch.GetId()), deref(ch.GetDisplayName())})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "NAME", "MEMBERSHIP"}
			rows := make([][]string, 0, len(channels))
			for _, ch := range channels {
				membership := ""
				if ch.GetMembershipType() != nil {
					membership = ch.GetMembershipType().String()
				}
				rows = append(rows, []string{deref(ch.GetId()), deref(ch.GetDisplayName()), membership})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  teams channels send
// ──────────────────────────────────────────────

var teamsChannelsSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a Teams channel",
	Long: `Send a message to a Teams channel.

By default the body is sent as plain text. Pass --html to send the body as
HTML (Teams supports a limited HTML subset: p, br, b, i, a, ul/ol/li,
blockquote, pre, code, img, and simple tables). HTML comments (<!--) are
rejected because they can hide the appended audit footer.

Safety: Requires --confirm flag to prevent accidental broadcast to channels.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		teamFlag, _ := cmd.Flags().GetString("team")
		channelFlag, _ := cmd.Flags().GetString("channel")
		bodyFlag, _ := cmd.Flags().GetString("body")
		confirmFlag, _ := cmd.Flags().GetBool("confirm")
		htmlFlag, _ := cmd.Flags().GetBool("html")

		if teamFlag == "" {
			return fmt.Errorf("--team is required")
		}
		if channelFlag == "" {
			return fmt.Errorf("--channel is required")
		}
		if bodyFlag == "" {
			return fmt.Errorf("--body is required")
		}
		if htmlFlag {
			if err := htmlBodyGuard(bodyFlag); err != nil {
				return err
			}
		}

		// Safety: require --confirm for channel posts
		if !confirmFlag {
			return fmt.Errorf("channel messages are visible to all members — pass --confirm to send")
		}

		cfg, profileName, profile, err := loadSelectedProfile()
		if err != nil {
			return err
		}
		client, err := newGraphClientForProfile(cfg, profileName, profile)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		teamID, teamName, err := resolveTeamID(ctx, client, teamFlag)
		if err != nil {
			return err
		}

		channelID, channelName, err := resolveChannelID(ctx, client, teamID, channelFlag)
		if err != nil {
			return err
		}

		// Safety: warn if channel has >10 members (large audience guard)
		members, membErr := client.Teams().ByTeamId(teamID).Members().Get(ctx, nil)
		if membErr == nil && members.GetValue() != nil && len(members.GetValue()) > 10 {
			output.Info(fmt.Sprintf("⚠ Team %q has %d members — this message will be visible to all of them", teamName, len(members.GetValue())))
		}

		if flagDryRun {
			if htmlFlag {
				output.Info(fmt.Sprintf("[DRY RUN] Would send HTML message to #%s in %s (%d chars)", channelName, teamName, len(bodyFlag)))
			} else {
				output.Info(fmt.Sprintf("[DRY RUN] Would send message to #%s in %s (%d chars)", channelName, teamName, len(bodyFlag)))
			}
			return nil
		}

		// Audit & Identity: tag all agent-generated messages with disclaimer
		taggedBody, contentType := taggedTeamsBody(bodyFlag, htmlFlag)

		msg := models.NewChatMessage()
		body := models.NewItemBody()
		body.SetContentType(&contentType)
		body.SetContent(&taggedBody)
		msg.SetBody(body)

		// Build request config with empty options to avoid nil pointer
		requestConfig := &teamsPkg.ItemChannelsItemMessagesRequestBuilderPostRequestConfiguration{}

		sent, err := client.Teams().ByTeamId(teamID).Channels().ByChannelId(channelID).Messages().Post(ctx, msg, requestConfig)
		if err != nil {
			return fmt.Errorf("sending channel message: %w", err)
		}
		if sent == nil || deref(sent.GetId()) == "" {
			return fmt.Errorf("message send returned no root-message identifier; deletion provenance was not recorded")
		}
		if profile.AuthMode == config.AuthModeDelegated && profile.ManagedDelegated != nil {
			target := auth.ManagedChannelMessageTarget{
				TeamID:    teamID,
				ChannelID: channelID,
				MessageID: deref(sent.GetId()),
			}
			provenanceCtx, provenanceCancel := context.WithTimeout(context.Background(), 30*time.Second)
			reference, provenanceErr := auth.RecordManagedChannelMessageProvenance(provenanceCtx, profile, target)
			provenanceCancel()
			if provenanceErr != nil {
				return fmt.Errorf("message was sent but managed deletion provenance was not recorded; do not use delete-message: %w", provenanceErr)
			}
			if profile.ManagedDelegated.ChannelMessageProvenance == nil {
				profile.ManagedDelegated.ChannelMessageProvenance = make(map[string]string)
			}
			key := auth.ManagedChannelMessageProvenanceKey(target)
			profile.ManagedDelegated.ChannelMessageProvenance[key] = reference
			if saveErr := cfg.Save(); saveErr != nil {
				deleteErr := auth.DeleteManagedChannelMessageProvenance(context.Background(), profile, reference, target)
				if deleteErr != nil {
					return fmt.Errorf("message was sent but deletion provenance metadata could not be saved or rolled back; do not use delete-message")
				}
				return fmt.Errorf("message was sent but deletion provenance metadata could not be saved; do not use delete-message: %w", saveErr)
			}
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":        deref(sent.GetId()),
				"team":      teamName,
				"channel":   channelName,
				"createdAt": sent.GetCreatedDateTime(),
			})
		default:
			output.Success(fmt.Sprintf("Message sent to #%s in %s (id: %s)", channelName, teamName, deref(sent.GetId())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  teams chat list
// ──────────────────────────────────────────────

var teamsChatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Teams chats",
	RunE: func(cmd *cobra.Command, args []string) error {
		maxFlag, _ := cmd.Flags().GetInt("max")
		if maxFlag <= 0 {
			maxFlag = 25
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		top := int32(maxFlag)
		config := &usersPkg.ItemChatsRequestBuilderGetRequestConfiguration{
			QueryParameters: &usersPkg.ItemChatsRequestBuilderGetQueryParameters{
				Top:     &top,
				Orderby: []string{"lastMessagePreview/createdDateTime desc"},
			},
		}

		result, err := client.Me().Chats().Get(ctx, config)
		if err != nil {
			return fmt.Errorf("listing chats: %w", err)
		}

		chats := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(chats))
			for _, ch := range chats {
				item := map[string]interface{}{
					"id":       deref(ch.GetId()),
					"topic":    deref(ch.GetTopic()),
					"chatType": "",
				}
				if ch.GetChatType() != nil {
					item["chatType"] = ch.GetChatType().String()
				}
				if ch.GetLastUpdatedDateTime() != nil {
					item["lastUpdated"] = ch.GetLastUpdatedDateTime().Format(time.RFC3339)
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(chats))
			for _, ch := range chats {
				rows = append(rows, []string{deref(ch.GetId()), deref(ch.GetTopic())})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "TYPE", "TOPIC", "LAST UPDATED"}
			rows := make([][]string, 0, len(chats))
			for _, ch := range chats {
				chatType := ""
				if ch.GetChatType() != nil {
					chatType = ch.GetChatType().String()
				}
				topic := deref(ch.GetTopic())
				if topic == "" {
					topic = "(no topic)"
				}
				lastUpdated := ""
				if ch.GetLastUpdatedDateTime() != nil {
					lastUpdated = ch.GetLastUpdatedDateTime().Format("2006-01-02 15:04")
				}
				rows = append(rows, []string{deref(ch.GetId()), chatType, topic, lastUpdated})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  teams chat send
// ──────────────────────────────────────────────

var teamsChatSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message in a Teams chat",
	RunE: func(cmd *cobra.Command, args []string) error {
		chatFlag, _ := cmd.Flags().GetString("chat")
		bodyFlag, _ := cmd.Flags().GetString("body")
		confirmFlag, _ := cmd.Flags().GetBool("confirm")

		if chatFlag == "" {
			return fmt.Errorf("--chat is required")
		}
		if bodyFlag == "" {
			return fmt.Errorf("--body is required")
		}
		if !confirmFlag && !flagDryRun {
			return fmt.Errorf("chat messages are visible to chat members — pass --confirm to send")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would send message to chat %s (%d chars)", chatFlag, len(bodyFlag)))
			return nil
		}

		// Audit & Identity: tag all agent-generated messages with disclaimer.
		// Chat send is text-only by design — --html is scoped to channel send (#20).
		taggedBody, contentType := taggedTeamsBody(bodyFlag, false)

		msg := models.NewChatMessage()
		body := models.NewItemBody()
		body.SetContentType(&contentType)
		body.SetContent(&taggedBody)
		msg.SetBody(body)

		config := &chatsPkg.ItemMessagesRequestBuilderPostRequestConfiguration{}

		sent, err := client.Chats().ByChatId(chatFlag).Messages().Post(ctx, msg, config)
		if err != nil {
			return fmt.Errorf("sending chat message: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":        deref(sent.GetId()),
				"chatId":    chatFlag,
				"createdAt": sent.GetCreatedDateTime(),
			})
		default:
			output.Success(fmt.Sprintf("Message sent to chat %s (id: %s)", chatFlag, deref(sent.GetId())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  Registration
// ──────────────────────────────────────────────

func init() {
	// teams channels list
	teamsChannelsListCmd.Flags().String("team", "", "Team name or ID (required)")
	teamsChannelsCmd.AddCommand(teamsChannelsListCmd)

	// teams channels send
	teamsChannelsSendCmd.Flags().String("team", "", "Team name or ID (required)")
	teamsChannelsSendCmd.Flags().String("channel", "", "Channel name or ID (required)")
	teamsChannelsSendCmd.Flags().String("body", "", "Message body text (required)")
	teamsChannelsSendCmd.Flags().Bool("html", false, "Send body as HTML instead of plain text")
	teamsChannelsSendCmd.Flags().Bool("confirm", false, "Confirm sending to channel (required safety flag)")
	teamsChannelsCmd.AddCommand(teamsChannelsSendCmd)

	// teams channels delete-message
	teamsChannelsDeleteMessageCmd.Flags().String("team", "", "Exact team ID (required)")
	teamsChannelsDeleteMessageCmd.Flags().String("channel", "", "Exact channel ID (required)")
	teamsChannelsDeleteMessageCmd.Flags().String("message", "", "Exact root message ID (required)")
	teamsChannelsDeleteMessageCmd.Flags().Bool("confirm", false, "Confirm soft-deleting this exact message (required)")
	teamsChannelsCmd.AddCommand(teamsChannelsDeleteMessageCmd)

	// teams chat list
	teamsChatListCmd.Flags().Int("max", 25, "Maximum chats to return")
	teamsChatCmd.AddCommand(teamsChatListCmd)

	// teams chat send
	teamsChatSendCmd.Flags().String("chat", "", "Chat ID (required)")
	teamsChatSendCmd.Flags().String("body", "", "Message body text (required)")
	teamsChatSendCmd.Flags().Bool("confirm", false, "Confirm sending to the chat (required safety flag)")
	teamsChatCmd.AddCommand(teamsChatSendCmd)

	// Wire up
	teamsCmd.AddCommand(teamsChannelsCmd)
	teamsCmd.AddCommand(teamsChatCmd)
}
