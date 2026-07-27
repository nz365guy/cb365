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
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("loop pages get missing --format flag")
	}
	if formatFlag.DefValue != loopPageFormatOriginal {
		t.Errorf("--format default should be %q", loopPageFormatOriginal)
	}
}

func TestLoopPageContentRequestConfiguration(t *testing.T) {
	t.Run("original leaves conversion query unset", func(t *testing.T) {
		config, selectedFormat, err := loopPageContentRequestConfiguration(loopPageFormatOriginal)
		if err != nil {
			t.Fatalf("original format returned error: %v", err)
		}
		if config != nil {
			t.Fatal("original format should not set request configuration")
		}
		if selectedFormat != loopPageFormatOriginal {
			t.Errorf("selected format = %q, want %q", selectedFormat, loopPageFormatOriginal)
		}
	})

	t.Run("html sets exact conversion query", func(t *testing.T) {
		config, selectedFormat, err := loopPageContentRequestConfiguration(loopPageFormatHTML)
		if err != nil {
			t.Fatalf("html format returned error: %v", err)
		}
		if config == nil || config.QueryParameters == nil || config.QueryParameters.Format == nil {
			t.Fatal("html format should set the conversion query")
		}
		if got := *config.QueryParameters.Format; got != loopPageFormatHTML {
			t.Errorf("conversion query format = %q, want %q", got, loopPageFormatHTML)
		}
		if selectedFormat != loopPageFormatHTML {
			t.Errorf("selected format = %q, want %q", selectedFormat, loopPageFormatHTML)
		}
	})

	t.Run("format is case normalised", func(t *testing.T) {
		config, selectedFormat, err := loopPageContentRequestConfiguration(" HTML ")
		if err != nil {
			t.Fatalf("normalised HTML format returned error: %v", err)
		}
		if config == nil || selectedFormat != loopPageFormatHTML {
			t.Fatalf("normalised format = %q with config %v", selectedFormat, config)
		}
	})

	t.Run("unsupported format is rejected", func(t *testing.T) {
		config, selectedFormat, err := loopPageContentRequestConfiguration("pdf")
		if err == nil {
			t.Fatal("unsupported format should return an error")
		}
		if config != nil || selectedFormat != "" {
			t.Fatalf("unsupported format returned config %v and format %q", config, selectedFormat)
		}
		want := `unsupported --format "pdf" (supported values: original, html)`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
	})
}

func TestResolveWorkspaceID(t *testing.T) {
	cfg := &loopConfig{
		Workspaces: []loopWorkspace{
			{ID: "b!abc123", Name: "Contoso", DisplayName: "Contoso"},
			{ID: "b!def456", Name: "Pages", DisplayName: "Pages (user)", Owner: "user@example.com"},
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
