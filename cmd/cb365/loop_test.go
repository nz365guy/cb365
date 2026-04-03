package main

import (
	"testing"
)

// ──────────────────────────────────────────────
//  Loop safety and structure tests
// ──────────────────────────────────────────────

func TestLoopCommandStructure(t *testing.T) {
	if !loopCmd.HasSubCommands() {
		t.Fatal("loop command should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range loopCmd.Commands() {
		found[sub.Name()] = true
	}

	if !found["pages"] {
		t.Error("loop missing subcommand 'pages'")
	}
}

func TestLoopPagesCommandStructure(t *testing.T) {
	if !loopPagesCmd.HasSubCommands() {
		t.Fatal("loop pages should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range loopPagesCmd.Commands() {
		found[sub.Name()] = true
	}

	for _, expected := range []string{"list", "get", "create"} {
		if !found[expected] {
			t.Errorf("loop pages missing subcommand %q", expected)
		}
	}
}

func TestLoopPagesListRequiresWorkspace(t *testing.T) {
	cmd := loopPagesListCmd
	wsFlag := cmd.Flags().Lookup("workspace")
	if wsFlag == nil {
		t.Fatal("loop pages list missing --workspace flag")
	}
}

func TestLoopPagesGetRequiresWorkspaceAndPage(t *testing.T) {
	cmd := loopPagesGetCmd
	wsFlag := cmd.Flags().Lookup("workspace")
	if wsFlag == nil {
		t.Fatal("loop pages get missing --workspace flag")
	}
	pageFlag := cmd.Flags().Lookup("page")
	if pageFlag == nil {
		t.Fatal("loop pages get missing --page flag")
	}
}

func TestLoopPagesCreateRequiresWorkspaceAndTitle(t *testing.T) {
	cmd := loopPagesCreateCmd
	wsFlag := cmd.Flags().Lookup("workspace")
	if wsFlag == nil {
		t.Fatal("loop pages create missing --workspace flag")
	}
	titleFlag := cmd.Flags().Lookup("title")
	if titleFlag == nil {
		t.Fatal("loop pages create missing --title flag")
	}
}

