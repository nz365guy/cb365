package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	teamsPkg "github.com/microsoftgraph/msgraph-sdk-go/teams"
	chatsPkg "github.com/microsoftgraph/msgraph-sdk-go/chats"
	usersPkg "github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

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

Safety: Requires --confirm flag to prevent accidental broadcast to channels.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		teamFlag, _ := cmd.Flags().GetString("team")
		channelFlag, _ := cmd.Flags().GetString("channel")
		bodyFlag, _ := cmd.Flags().GetString("body")
		confirmFlag, _ := cmd.Flags().GetBool("confirm")

		if teamFlag == "" {
			return fmt.Errorf("--team is required")
		}
		if channelFlag == "" {
			return fmt.Errorf("--channel is required")
		}
		if bodyFlag == "" {
			return fmt.Errorf("--body is required")
		}

		// Safety: require --confirm for channel posts
		if !confirmFlag {
			return fmt.Errorf("channel messages are visible to all members — pass --confirm to send")
		}

		client, err := newGraphClient()
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

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would send message to #%s in %s (%d chars)", channelName, teamName, len(bodyFlag)))
			return nil
		}

		msg := models.NewChatMessage()
		body := models.NewItemBody()
		contentType := models.TEXT_BODYTYPE
		body.SetContentType(&contentType)
		body.SetContent(&bodyFlag)
		msg.SetBody(body)

		// Build request config with empty options to avoid nil pointer
		config := &teamsPkg.ItemChannelsItemMessagesRequestBuilderPostRequestConfiguration{}

		sent, err := client.Teams().ByTeamId(teamID).Channels().ByChannelId(channelID).Messages().Post(ctx, msg, config)
		if err != nil {
			return fmt.Errorf("sending channel message: %w", err)
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
				Top: &top,
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

		if chatFlag == "" {
			return fmt.Errorf("--chat is required")
		}
		if bodyFlag == "" {
			return fmt.Errorf("--body is required")
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

		msg := models.NewChatMessage()
		body := models.NewItemBody()
		contentType := models.TEXT_BODYTYPE
		body.SetContentType(&contentType)
		body.SetContent(&bodyFlag)
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
	teamsChannelsSendCmd.Flags().Bool("confirm", false, "Confirm sending to channel (required safety flag)")
	teamsChannelsCmd.AddCommand(teamsChannelsSendCmd)

	// teams chat list
	teamsChatListCmd.Flags().Int("max", 25, "Maximum chats to return")
	teamsChatCmd.AddCommand(teamsChatListCmd)

	// teams chat send
	teamsChatSendCmd.Flags().String("chat", "", "Chat ID (required)")
	teamsChatSendCmd.Flags().String("body", "", "Message body text (required)")
	teamsChatCmd.AddCommand(teamsChatSendCmd)

	// Wire up
	teamsCmd.AddCommand(teamsChannelsCmd)
	teamsCmd.AddCommand(teamsChatCmd)
}

