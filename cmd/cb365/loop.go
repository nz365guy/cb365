package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	drivesPkg "github.com/microsoftgraph/msgraph-sdk-go/drives"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Loop — workspaces via SharePoint Embedded
//
//  Microsoft Loop stores workspaces as SharePoint
//  Embedded (SPE) file storage containers. The
//  container listing API requires SPE admin setup,
//  so workspace IDs are stored in a local config
//  file populated via PowerShell discovery.
//
//  Page access uses the standard Graph drives API:
//    GET /drives/{containerId}/root/children
//    GET /drives/{containerId}/items/{itemId}/content
//
//  IMPORTANT: Loop requires app-only auth (--profile
//  work-app) due to SPE guest app permissions being
//  app-only. The --profile flag is auto-set if not
//  specified.
// ──────────────────────────────────────────────

// loopWorkspace represents a Loop workspace from config.
type loopWorkspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Type        string `json:"type,omitempty"`
}

// loopConfig holds Loop workspace configuration.
type loopConfig struct {
	Workspaces []loopWorkspace `json:"workspaces"`
}

// loopConfigPath returns the path to the Loop workspaces config file.
func loopConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cb365", "loop-workspaces.json")
}

// loadLoopConfig reads the Loop workspaces config file.
func loadLoopConfig() (*loopConfig, error) {
	path := loopConfigPath()
	data, err := os.ReadFile(path) // #nosec G304 — config file path
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w\n\nRun 'cb365 loop workspaces import' to populate from PowerShell", path, err)
	}
	var cfg loopConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// resolveWorkspaceID resolves a workspace name or ID.
func resolveWorkspaceID(cfg *loopConfig, nameOrID string) (*loopWorkspace, error) {
	// Try direct ID match
	for i, ws := range cfg.Workspaces {
		if ws.ID == nameOrID {
			return &cfg.Workspaces[i], nil
		}
	}
	// Try name match (case-insensitive)
	target := strings.ToLower(nameOrID)
	for i, ws := range cfg.Workspaces {
		if strings.ToLower(ws.Name) == target || strings.ToLower(ws.DisplayName) == target {
			return &cfg.Workspaces[i], nil
		}
	}
	return nil, fmt.Errorf("workspace %q not found — run 'cb365 loop workspaces list' to see available workspaces", nameOrID)
}

// ensureLoopProfile sets the profile to work-app if not explicitly set.
// Loop requires app-only auth due to SPE guest app permissions.
func ensureLoopProfile() {
	if flagProfile == "" {
		flagProfile = "work-app"
	}
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Microsoft Loop — workspaces and pages",
	Long: `Microsoft Loop operations via SharePoint Embedded containers.

Loop workspaces are stored as SPE file storage containers. Workspace
discovery uses a local config file populated via PowerShell. Page access
uses the standard Graph drives API with app-only auth.

Note: Loop commands automatically use the work-app profile (app-only auth)
unless --profile is explicitly set.`,
}

var loopWorkspacesCmd = &cobra.Command{
	Use:   "workspaces",
	Short: "Manage Loop workspace registry",
}

var loopPagesCmd = &cobra.Command{
	Use:   "pages",
	Short: "List and download Loop pages",
}

// ──────────────────────────────────────────────
//  loop workspaces list
// ──────────────────────────────────────────────

var loopWorkspacesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known Loop workspaces from local config",
	Long: `List Loop workspaces stored in the local config file.

Workspaces are discovered via SharePoint PowerShell and stored locally.
Run 'cb365 loop workspaces import' to refresh the list.

Examples:
  cb365 loop workspaces list
  cb365 loop workspaces list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadLoopConfig()
		if err != nil {
			return err
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(cfg.Workspaces)
		case output.FormatPlain:
			rows := make([][]string, 0, len(cfg.Workspaces))
			for _, ws := range cfg.Workspaces {
				rows = append(rows, []string{ws.ID, ws.Name})
			}
			output.Plain(rows)
		default:
			headers := []string{"NAME", "TYPE", "OWNER"}
			rows := make([][]string, 0, len(cfg.Workspaces))
			for _, ws := range cfg.Workspaces {
				name := ws.DisplayName
				if name == "" {
					name = ws.Name
				}
				owner := ws.Owner
				if owner == "" {
					owner = "-"
				}
				rows = append(rows, []string{name, ws.Type, owner})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  loop pages list
// ──────────────────────────────────────────────

var loopPagesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pages in a Loop workspace",
	Long: `List pages (.loop files) in a Loop workspace.

The workspace can be specified by name or container ID.
Uses app-only auth via the Graph drives API.

Examples:
  cb365 loop pages list --workspace Cloverbase
  cb365 loop pages list --workspace "Microsoft Innovation Podcast" --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		folderFlag, _ := cmd.Flags().GetString("folder")
		if wsFlag == "" {
			return fmt.Errorf("--workspace is required (name or container ID)")
		}

		ensureLoopProfile()

		cfg, err := loadLoopConfig()
		if err != nil {
			return err
		}

		ws, err := resolveWorkspaceID(cfg, wsFlag)
		if err != nil {
			return err
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Determine which folder to list
		parentID := "root"
		if folderFlag != "" {
			parentID = folderFlag
		}

		// List children via standard drives API
		listConfig := &drivesPkg.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{}
		result, err := client.Drives().ByDriveId(ws.ID).Items().ByDriveItemId(parentID).Children().Get(ctx, listConfig)
		if err != nil {
			return fmt.Errorf("listing pages in %q: %w", ws.Name, err)
		}

		driveItems := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(driveItems))
			for _, item := range driveItems {
				name := deref(item.GetName())
				entry := map[string]interface{}{
					"id":       deref(item.GetId()),
					"name":     name,
					"isLoop":   strings.HasSuffix(name, ".loop") || strings.HasSuffix(name, ".fluid"),
					"isFolder": item.GetFolder() != nil,
				}
				if item.GetSize() != nil {
					entry["size"] = *item.GetSize()
				}
				if item.GetLastModifiedDateTime() != nil {
					entry["lastModified"] = item.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if item.GetFolder() != nil && item.GetFolder().GetChildCount() != nil {
					entry["childCount"] = *item.GetFolder().GetChildCount()
				}
				items = append(items, entry)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(driveItems))
			for _, item := range driveItems {
				rows = append(rows, []string{deref(item.GetId()), deref(item.GetName())})
			}
			output.Plain(rows)
		default:
			headers := []string{"TYPE", "NAME", "SIZE", "LAST MODIFIED"}
			rows := make([][]string, 0, len(driveItems))
			for _, item := range driveItems {
				name := deref(item.GetName())
				typeStr := "📄"
				sizeStr := ""
				if item.GetSize() != nil {
					sizeStr = humanFileSize(*item.GetSize())
				}
				if item.GetFolder() != nil {
					typeStr = "📁"
					if item.GetFolder().GetChildCount() != nil {
						sizeStr = fmt.Sprintf("%d items", *item.GetFolder().GetChildCount())
					}
				}
				lastMod := ""
				if item.GetLastModifiedDateTime() != nil {
					lastMod = item.GetLastModifiedDateTime().Format("2006-01-02 15:04")
				}
				rows = append(rows, []string{typeStr, name, sizeStr, lastMod})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  loop pages get
// ──────────────────────────────────────────────

var loopPagesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Download a Loop page",
	Long: `Download a Loop page (.loop file) from a workspace.

Examples:
  cb365 loop pages get --workspace Cloverbase --page ITEM_ID --output ./page.loop
  cb365 loop pages get --workspace Cloverbase --page ITEM_ID     # prints to stdout`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		pageFlag, _ := cmd.Flags().GetString("page")
		outputFlag, _ := cmd.Flags().GetString("output")

		if wsFlag == "" {
			return fmt.Errorf("--workspace is required")
		}
		if pageFlag == "" {
			return fmt.Errorf("--page is required (item ID from 'cb365 loop pages list')")
		}

		ensureLoopProfile()

		cfg, err := loadLoopConfig()
		if err != nil {
			return err
		}

		ws, err := resolveWorkspaceID(cfg, wsFlag)
		if err != nil {
			return err
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if flagDryRun {
			target := outputFlag
			if target == "" {
				target = "stdout"
			}
			output.Info(fmt.Sprintf("[DRY RUN] Would download page %s from %q → %s", pageFlag, ws.Name, target))
			return nil
		}

		content, err := client.Drives().ByDriveId(ws.ID).Items().ByDriveItemId(pageFlag).Content().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("downloading page: %w", err)
		}

		if outputFlag != "" {
			dir := filepath.Dir(outputFlag)
			tmpFile, tmpErr := os.CreateTemp(dir, ".cb365-loop-*")
			if tmpErr != nil {
				return fmt.Errorf("creating temp file: %w", tmpErr)
			}
			tmpPath := tmpFile.Name()

			_, writeErr := tmpFile.Write(content)
			closeErr := tmpFile.Close()
			if writeErr != nil {
				os.Remove(tmpPath) // #nosec G104
				return fmt.Errorf("writing file: %w", writeErr)
			}
			if closeErr != nil {
				os.Remove(tmpPath) // #nosec G104
				return fmt.Errorf("closing temp file: %w", closeErr)
			}
			if err := os.Rename(tmpPath, outputFlag); err != nil {
				os.Remove(tmpPath) // #nosec G104
				return fmt.Errorf("moving temp file: %w", err)
			}

			format := output.Resolve(flagJSON, flagPlain)
			switch format {
			case output.FormatJSON:
				return output.JSON(map[string]interface{}{
					"path":      outputFlag,
					"size":      len(content),
					"workspace": ws.Name,
				})
			default:
				output.Success(fmt.Sprintf("Downloaded page → %s (%s)", outputFlag, humanFileSize(int64(len(content)))))
			}
		} else {
			_, _ = os.Stdout.Write(content)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  Registration
// ──────────────────────────────────────────────

func init() {
	// loop workspaces list
	loopWorkspacesCmd.AddCommand(loopWorkspacesListCmd)

	// loop pages list
	loopPagesListCmd.Flags().String("workspace", "", "Workspace name or container ID (required)")
	loopPagesListCmd.Flags().String("folder", "", "Folder item ID to list (default: root)")
	loopPagesCmd.AddCommand(loopPagesListCmd)

	// loop pages get
	loopPagesGetCmd.Flags().String("workspace", "", "Workspace name or container ID (required)")
	loopPagesGetCmd.Flags().String("page", "", "Page item ID (required)")
	loopPagesGetCmd.Flags().String("output", "", "Output file path (omit for stdout)")
	loopPagesCmd.AddCommand(loopPagesGetCmd)

	// Wire up
	loopCmd.AddCommand(loopWorkspacesCmd)
	loopCmd.AddCommand(loopPagesCmd)
}

