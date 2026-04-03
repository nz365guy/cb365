package main

import (
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
//  Teams safety rule tests
// ──────────────────────────────────────────────

func TestTeamsChannelSendRequiresConfirm(t *testing.T) {
	// Safety: channel send must require --confirm
	cmd := teamsChannelsSendCmd
	confirmFlag := cmd.Flags().Lookup("confirm")
	if confirmFlag == nil {
		t.Fatal("teams channels send missing --confirm flag")
	}
	if confirmFlag.DefValue != "false" {
		t.Errorf("--confirm default should be false, got %s", confirmFlag.DefValue)
	}
}

func TestTeamsChannelSendRequiresTeam(t *testing.T) {
	cmd := teamsChannelsSendCmd
	teamFlag := cmd.Flags().Lookup("team")
	if teamFlag == nil {
		t.Fatal("teams channels send missing --team flag")
	}
}

func TestTeamsChannelSendRequiresChannel(t *testing.T) {
	cmd := teamsChannelsSendCmd
	channelFlag := cmd.Flags().Lookup("channel")
	if channelFlag == nil {
		t.Fatal("teams channels send missing --channel flag")
	}
}

func TestTeamsChannelSendRequiresBody(t *testing.T) {
	cmd := teamsChannelsSendCmd
	bodyFlag := cmd.Flags().Lookup("body")
	if bodyFlag == nil {
		t.Fatal("teams channels send missing --body flag")
	}
}

func TestTeamsChatSendRequiresChat(t *testing.T) {
	cmd := teamsChatSendCmd
	chatFlag := cmd.Flags().Lookup("chat")
	if chatFlag == nil {
		t.Fatal("teams chat send missing --chat flag")
	}
}

func TestTeamsChatSendRequiresBody(t *testing.T) {
	cmd := teamsChatSendCmd
	bodyFlag := cmd.Flags().Lookup("body")
	if bodyFlag == nil {
		t.Fatal("teams chat send missing --body flag")
	}
}

func TestTeamsChatListMaxDefault(t *testing.T) {
	cmd := teamsChatListCmd
	maxFlag := cmd.Flags().Lookup("max")
	if maxFlag == nil {
		t.Fatal("teams chat list missing --max flag")
	}
	if maxFlag.DefValue != "25" {
		t.Errorf("--max default should be 25, got %s", maxFlag.DefValue)
	}
}

func TestTeamsChannelsListRequiresTeam(t *testing.T) {
	cmd := teamsChannelsListCmd
	teamFlag := cmd.Flags().Lookup("team")
	if teamFlag == nil {
		t.Fatal("teams channels list missing --team flag")
	}
}

func TestTeamsCommandStructure(t *testing.T) {
	// Verify command hierarchy
	if !teamsCmd.HasSubCommands() {
		t.Fatal("teams command should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range teamsCmd.Commands() {
		found[sub.Name()] = true
	}

	for _, expected := range []string{"channels", "chat"} {
		if !found[expected] {
			t.Errorf("teams missing subcommand %q", expected)
		}
	}
}


func TestTeamsAuditFooterExists(t *testing.T) {
	if teamsAuditFooter == "" {
		t.Fatal("teamsAuditFooter must not be empty")
	}
	if !strings.Contains(teamsAuditFooter, "cb365") {
		t.Error("teamsAuditFooter must contain 'cb365' identifier")
	}
}
