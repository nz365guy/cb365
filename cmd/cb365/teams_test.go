package main

import (
	"strings"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
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

// ──────────────────────────────────────────────
//  --html flag tests (#20)
// ──────────────────────────────────────────────

func TestTeamsChannelSendHasHTMLFlag(t *testing.T) {
	cmd := teamsChannelsSendCmd
	htmlFlag := cmd.Flags().Lookup("html")
	if htmlFlag == nil {
		t.Fatal("teams channels send missing --html flag")
	}
	// Default text behaviour must remain unchanged
	if htmlFlag.DefValue != "false" {
		t.Errorf("--html default should be false, got %s", htmlFlag.DefValue)
	}
}

func TestTeamsChatSendHasNoHTMLFlag(t *testing.T) {
	// Scope guard: #20 approved --html for channel send only
	cmd := teamsChatSendCmd
	if cmd.Flags().Lookup("html") != nil {
		t.Error("teams chat send must not have --html — scoped to channels send only")
	}
}

func TestTeamsAuditFooterHTMLAttribution(t *testing.T) {
	if teamsAuditFooterHTML == "" {
		t.Fatal("teamsAuditFooterHTML must not be empty")
	}
	if !strings.Contains(teamsAuditFooterHTML, "cb365") {
		t.Error("teamsAuditFooterHTML must contain 'cb365' identifier")
	}
	if !strings.Contains(teamsAuditFooterHTML, "<br>") {
		t.Error("teamsAuditFooterHTML must use <br> — plain newlines do not render in HTML bodies")
	}
	// Both footers must carry the same visible attribution text
	visible := strings.TrimSpace(teamsAuditFooter)
	if !strings.Contains(teamsAuditFooterHTML, visible) {
		t.Errorf("teamsAuditFooterHTML must contain the visible attribution %q", visible)
	}
}

func TestTaggedTeamsBodyText(t *testing.T) {
	content, contentType := taggedTeamsBody("hello", false)
	if content != "hello"+teamsAuditFooter {
		t.Errorf("text body should be body + teamsAuditFooter, got %q", content)
	}
	if contentType != models.TEXT_BODYTYPE {
		t.Errorf("text mode should use TEXT_BODYTYPE, got %v", contentType)
	}
}

func TestTaggedTeamsBodyHTML(t *testing.T) {
	content, contentType := taggedTeamsBody("<b>hello</b>", true)
	if content != "<b>hello</b>"+teamsAuditFooterHTML {
		t.Errorf("HTML body should be body + teamsAuditFooterHTML, got %q", content)
	}
	if contentType != models.HTML_BODYTYPE {
		t.Errorf("HTML mode should use HTML_BODYTYPE, got %v", contentType)
	}
}

func TestHTMLBodyGuardRejectsComments(t *testing.T) {
	// An unterminated <!-- swallows the appended audit footer when the body
	// is parsed as HTML, so comment openers must be rejected.
	if err := htmlBodyGuard("hidden <!--"); err == nil {
		t.Fatal("HTML body containing <!-- must be rejected")
	} else if !strings.Contains(err.Error(), "audit footer") {
		t.Errorf("rejection should explain the audit-footer risk, got: %v", err)
	}
	if err := htmlBodyGuard("<b>fine</b>"); err != nil {
		t.Errorf("clean HTML body should pass, got: %v", err)
	}
}
