package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestParseRFC3339StrictRejectsShortInputWithoutPanic(t *testing.T) {
	for _, value := range []string{"", "x", "2026-01-01"} {
		if _, err := parseRFC3339Strict(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestPrivateCalendarPreviewIsRedacted(t *testing.T) {
	event := models.NewEvent()
	private := models.PRIVATE_SENSITIVITY
	preview := "private appointment detail"
	event.SetSensitivity(&private)
	event.SetBodyPreview(&preview)
	if _, ok := formatEventJSON(event)["body_preview"]; ok {
		t.Fatal("private event body preview must not be emitted")
	}
}

func TestContactHomePhonesRequirePrivateOptIn(t *testing.T) {
	contact := models.NewContact()
	contact.SetHomePhones([]string{"synthetic-home-number"})
	if _, ok := formatContactJSON(contact, false)["home_phones"]; ok {
		t.Fatal("home phones must be omitted by default")
	}
	if _, ok := formatContactJSON(contact, true)["home_phones"]; !ok {
		t.Fatal("home phones should be present after explicit private opt-in")
	}
}

func TestWorkspaceNameAmbiguityFailsClosed(t *testing.T) {
	cfg := &loopConfig{Workspaces: []loopWorkspace{{ID: "a", Name: "Notes"}, {ID: "b", DisplayName: "notes"}}}
	if _, err := resolveWorkspaceID(cfg, "NOTES"); err == nil {
		t.Fatal("ambiguous workspace name must require an exact ID")
	}
}

func TestCommitTempFileDoesNotOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "result.txt")
	tmp := filepath.Join(dir, "temporary.txt")
	if err := os.WriteFile(destination, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := commitTempFile(tmp, destination, false); err == nil {
		t.Fatal("exclusive publish should reject an existing destination")
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "original" {
		t.Fatalf("destination was modified: %q, err %v", got, err)
	}
}

func TestTeamsChatSendRequiresConfirm(t *testing.T) {
	flag := teamsChatSendCmd.Flags().Lookup("confirm")
	if flag == nil || flag.DefValue != "false" {
		t.Fatal("teams chat send must have a default-false --confirm flag")
	}
}
