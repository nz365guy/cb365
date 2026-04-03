package main

import (
	"testing"
)

func TestLoopCommandStructure(t *testing.T) {
	if !loopCmd.HasSubCommands() {
		t.Fatal("loop command should have subcommands")
	}
	found := map[string]bool{}
	for _, sub := range loopCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"workspaces", "pages"} {
		if !found[expected] {
			t.Errorf("loop missing subcommand %q", expected)
		}
	}
}

func TestLoopPagesListRequiresWorkspace(t *testing.T) {
	cmd := loopPagesListCmd
	if cmd.Flags().Lookup("workspace") == nil {
		t.Fatal("loop pages list missing --workspace flag")
	}
}

func TestLoopPagesGetRequiresWorkspaceAndPage(t *testing.T) {
	cmd := loopPagesGetCmd
	if cmd.Flags().Lookup("workspace") == nil {
		t.Fatal("loop pages get missing --workspace flag")
	}
	if cmd.Flags().Lookup("page") == nil {
		t.Fatal("loop pages get missing --page flag")
	}
	if cmd.Flags().Lookup("output") == nil {
		t.Fatal("loop pages get missing --output flag")
	}
}

func TestResolveWorkspaceID(t *testing.T) {
	cfg := &loopConfig{
		Workspaces: []loopWorkspace{
			{ID: "b!abc123", Name: "Contoso", DisplayName: "Contoso"},
			{ID: "b!def456", Name: "Pages", DisplayName: "Pages (mark)", Owner: "mark@test.com"},
		},
	}

	// By ID
	ws, err := resolveWorkspaceID(cfg, "b!abc123")
	if err != nil || ws.Name != "Contoso" {
		t.Errorf("resolve by ID failed: %v", err)
	}

	// By name
	ws, err = resolveWorkspaceID(cfg, "Contoso")
	if err != nil || ws.ID != "b!abc123" {
		t.Errorf("resolve by name failed: %v", err)
	}

	// Case insensitive
	ws, err = resolveWorkspaceID(cfg, "contoso")
	if err != nil || ws.ID != "b!abc123" {
		t.Errorf("resolve case-insensitive failed: %v", err)
	}

	// Not found
	_, err = resolveWorkspaceID(cfg, "nonexistent")
	if err == nil {
		t.Error("resolve should fail for nonexistent workspace")
	}
}


func TestLoopPagesDeleteRequiresForce(t *testing.T) {
	cmd := loopPagesDeleteCmd
	if cmd.Flags().Lookup("force") == nil {
		t.Fatal("loop pages delete missing --force flag")
	}
	if cmd.Flags().Lookup("force").DefValue != "false" {
		t.Errorf("--force default should be false")
	}
}

func TestLoopPagesUploadRequiresFileAndPath(t *testing.T) {
	cmd := loopPagesUploadCmd
	if cmd.Flags().Lookup("file") == nil {
		t.Fatal("loop pages upload missing --file flag")
	}
	if cmd.Flags().Lookup("path") == nil {
		t.Fatal("loop pages upload missing --path flag")
	}
}

func TestLoopPagesMkdirRequiresPath(t *testing.T) {
	cmd := loopPagesMkdirCmd
	if cmd.Flags().Lookup("path") == nil {
		t.Fatal("loop pages mkdir missing --path flag")
	}
}

func TestLoopPagesFullCommandStructure(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range loopPagesCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"list", "get", "delete", "upload", "mkdir"} {
		if !found[expected] {
			t.Errorf("loop pages missing subcommand %q", expected)
		}
	}
}
