package main

import (
	"context"
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
//  OneDrive helpers
// ──────────────────────────────────────────────

// humanFileSize formats bytes as human-readable file size.
func humanFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var onedriveCmd = &cobra.Command{
	Use:     "onedrive",
	Aliases: []string{"od"},
	Short:   "OneDrive — files and folders",
}

// ──────────────────────────────────────────────
//  onedrive ls
// ──────────────────────────────────────────────

var onedriveLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List files and folders in OneDrive",
	Long: `List files and folders in OneDrive.

Examples:
  cb365 onedrive ls                    # List root
  cb365 onedrive ls --path "/Documents"
  cb365 onedrive ls --item-id ABC123   # By drive item ID`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pathFlag, _ := cmd.Flags().GetString("path")
		itemIDFlag, _ := cmd.Flags().GetString("item-id")

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get the user's drive ID first
		drive, err := client.Me().Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting user drive: %w", err)
		}
		driveID := deref(drive.GetId())

		type fileEntry struct {
			ID           string
			Name         string
			Size         int64
			IsFolder     bool
			LastModified string
			WebURL       string
			MimeType     string
			ChildCount   int32
		}

		var entries []fileEntry

		if itemIDFlag != "" {
			// List children of specific item by ID
			config := &drivesPkg.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{}
			result, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemIDFlag).Children().Get(ctx, config)
			if err != nil {
				return fmt.Errorf("listing folder contents: %w", err)
			}
			for _, item := range result.GetValue() {
				entry := fileEntry{
					ID:       deref(item.GetId()),
					Name:     deref(item.GetName()),
					WebURL:   deref(item.GetWebUrl()),
					IsFolder: item.GetFolder() != nil,
				}
				if item.GetSize() != nil {
					entry.Size = *item.GetSize()
				}
				if item.GetLastModifiedDateTime() != nil {
					entry.LastModified = item.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if item.GetFolder() != nil && item.GetFolder().GetChildCount() != nil {
					entry.ChildCount = *item.GetFolder().GetChildCount()
				}
				if item.GetFile() != nil && item.GetFile().GetMimeType() != nil {
					entry.MimeType = *item.GetFile().GetMimeType()
				}
				entries = append(entries, entry)
			}
		} else if pathFlag != "" && pathFlag != "/" {
			// List children by path — use URL-based approach via drive items
			// Graph API: /drives/{id}/root:/{path}:/children
			// The SDK uses ItemByPath which maps to /drives/{id}/items/root:/{path}
			cleanPath := strings.TrimPrefix(pathFlag, "/")
			itemByPath := fmt.Sprintf("root:/%s:", cleanPath)

			config := &drivesPkg.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{}
			result, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemByPath).Children().Get(ctx, config)
			if err != nil {
				return fmt.Errorf("listing path %q: %w", pathFlag, err)
			}
			for _, item := range result.GetValue() {
				entry := fileEntry{
					ID:       deref(item.GetId()),
					Name:     deref(item.GetName()),
					WebURL:   deref(item.GetWebUrl()),
					IsFolder: item.GetFolder() != nil,
				}
				if item.GetSize() != nil {
					entry.Size = *item.GetSize()
				}
				if item.GetLastModifiedDateTime() != nil {
					entry.LastModified = item.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if item.GetFolder() != nil && item.GetFolder().GetChildCount() != nil {
					entry.ChildCount = *item.GetFolder().GetChildCount()
				}
				if item.GetFile() != nil && item.GetFile().GetMimeType() != nil {
					entry.MimeType = *item.GetFile().GetMimeType()
				}
				entries = append(entries, entry)
			}
		} else {
			// List root children
			config := &drivesPkg.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{}
			result, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId("root").Children().Get(ctx, config)
			if err != nil {
				return fmt.Errorf("listing root: %w", err)
			}
			for _, item := range result.GetValue() {
				entry := fileEntry{
					ID:       deref(item.GetId()),
					Name:     deref(item.GetName()),
					WebURL:   deref(item.GetWebUrl()),
					IsFolder: item.GetFolder() != nil,
				}
				if item.GetSize() != nil {
					entry.Size = *item.GetSize()
				}
				if item.GetLastModifiedDateTime() != nil {
					entry.LastModified = item.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if item.GetFolder() != nil && item.GetFolder().GetChildCount() != nil {
					entry.ChildCount = *item.GetFolder().GetChildCount()
				}
				if item.GetFile() != nil && item.GetFile().GetMimeType() != nil {
					entry.MimeType = *item.GetFile().GetMimeType()
				}
				entries = append(entries, entry)
			}
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(entries))
			for _, e := range entries {
				item := map[string]interface{}{
					"id":       e.ID,
					"name":     e.Name,
					"size":     e.Size,
					"isFolder": e.IsFolder,
					"webUrl":   e.WebURL,
				}
				if e.LastModified != "" {
					item["lastModified"] = e.LastModified
				}
				if e.MimeType != "" {
					item["mimeType"] = e.MimeType
				}
				if e.IsFolder {
					item["childCount"] = e.ChildCount
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				typeStr := "file"
				if e.IsFolder {
					typeStr = "dir"
				}
				rows = append(rows, []string{e.ID, typeStr, e.Name, fmt.Sprintf("%d", e.Size)})
			}
			output.Plain(rows)
		default:
			headers := []string{"TYPE", "NAME", "SIZE", "LAST MODIFIED"}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				typeStr := "📄"
				sizeStr := humanFileSize(e.Size)
				if e.IsFolder {
					typeStr = "📁"
					sizeStr = fmt.Sprintf("%d items", e.ChildCount)
				}
				lastMod := ""
				if e.LastModified != "" {
					t, _ := time.Parse(time.RFC3339, e.LastModified)
					lastMod = t.Format("2006-01-02 15:04")
				}
				rows = append(rows, []string{typeStr, e.Name, sizeStr, lastMod})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  onedrive get (download)
// ──────────────────────────────────────────────

var onedriveGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Download a file from OneDrive",
	Long: `Download a file from OneDrive to a local path.

Safety: Downloads to a temp file first, then moves into place.
Will not overwrite existing files without --force.

Examples:
  cb365 onedrive get --path "/Documents/report.pdf" --output ./report.pdf
  cb365 onedrive get --item-id ABC123 --output ./file.txt`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pathFlag, _ := cmd.Flags().GetString("path")
		itemIDFlag, _ := cmd.Flags().GetString("item-id")
		outputFlag, _ := cmd.Flags().GetString("output")
		forceFlag, _ := cmd.Flags().GetBool("force")

		if pathFlag == "" && itemIDFlag == "" {
			return fmt.Errorf("--path or --item-id is required")
		}
		if outputFlag == "" {
			return fmt.Errorf("--output is required")
		}

		// Safety: check if output file exists
		if !forceFlag {
			if _, err := os.Stat(outputFlag); err == nil {
				return fmt.Errorf("output file %q already exists — use --force to overwrite", outputFlag)
			}
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Get the user's drive
		drive, err := client.Me().Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting user drive: %w", err)
		}
		driveID := deref(drive.GetId())

		if flagDryRun {
			target := pathFlag
			if target == "" {
				target = itemIDFlag
			}
			output.Info(fmt.Sprintf("[DRY RUN] Would download %s → %s", target, outputFlag))
			return nil
		}

		var content []byte

		if itemIDFlag != "" {
			content, err = client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemIDFlag).Content().Get(ctx, nil)
		} else {
			cleanPath := strings.TrimPrefix(pathFlag, "/")
			itemByPath := fmt.Sprintf("root:/%s:", cleanPath)
			content, err = client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemByPath).Content().Get(ctx, nil)
		}

		if err != nil {
			return fmt.Errorf("downloading file: %w", err)
		}

		// Safety: write to temp file first, then rename
		dir := filepath.Dir(outputFlag)
		tmpFile, err := os.CreateTemp(dir, ".cb365-download-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath := tmpFile.Name()

		_, writeErr := tmpFile.Write(content)
		closeErr := tmpFile.Close()
		if writeErr != nil {
			os.Remove(tmpPath) // #nosec G104 — cleanup best effort
			return fmt.Errorf("writing file: %w", writeErr)
		}
		if closeErr != nil {
			os.Remove(tmpPath) // #nosec G104 — cleanup best effort
			return fmt.Errorf("closing temp file: %w", closeErr)
		}

		if err := os.Rename(tmpPath, outputFlag); err != nil {
			os.Remove(tmpPath) // #nosec G104 — cleanup best effort
			return fmt.Errorf("moving temp file to %s: %w", outputFlag, err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"path": outputFlag,
				"size": len(content),
			})
		default:
			output.Success(fmt.Sprintf("Downloaded %s (%s)", outputFlag, humanFileSize(int64(len(content)))))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  onedrive upload
// ──────────────────────────────────────────────

var onedriveUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a file to OneDrive",
	Long: `Upload a local file to OneDrive.

Safety:
  - Validates file size before upload (max 4MB for simple upload)
  - --force required to overwrite existing files
  - --dry-run to preview

Examples:
  cb365 onedrive upload --file ./report.pdf --path "/Documents/report.pdf"
  cb365 onedrive upload --file ./data.csv --path "/Uploads/data.csv" --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fileFlag, _ := cmd.Flags().GetString("file")
		pathFlag, _ := cmd.Flags().GetString("path")
		forceFlag, _ := cmd.Flags().GetBool("force")

		if fileFlag == "" {
			return fmt.Errorf("--file is required (local file path)")
		}
		if pathFlag == "" {
			return fmt.Errorf("--path is required (OneDrive destination path)")
		}

		// Read the local file
		info, err := os.Stat(fileFlag)
		if err != nil {
			return fmt.Errorf("reading local file: %w", err)
		}

		// Safety: size limit for simple upload (4MB)
		const maxSimpleUpload = 4 * 1024 * 1024
		if info.Size() > maxSimpleUpload {
			return fmt.Errorf("file is %s — simple upload limit is 4MB. Large file upload not yet implemented", humanFileSize(info.Size()))
		}

		if info.Size() == 0 {
			return fmt.Errorf("file is empty — refusing to upload a 0-byte file")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Get the user's drive
		drive, err := client.Me().Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting user drive: %w", err)
		}
		driveID := deref(drive.GetId())

		// Check if target already exists (unless --force)
		if !forceFlag {
			cleanPath := strings.TrimPrefix(pathFlag, "/")
			itemByPath := fmt.Sprintf("root:/%s:", cleanPath)
			_, existErr := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemByPath).Get(ctx, nil)
			if existErr == nil {
				return fmt.Errorf("file already exists at %s — use --force to overwrite", pathFlag)
			}
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would upload %s (%s) → %s", fileFlag, humanFileSize(info.Size()), pathFlag))
			return nil
		}

		content, err := os.ReadFile(fileFlag) // #nosec G304 — user-specified file
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		// Upload via PUT /drives/{id}/items/root:/{path}:/content
		cleanPath := strings.TrimPrefix(pathFlag, "/")
		itemByPath := fmt.Sprintf("root:/%s:", cleanPath)

		uploaded, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemByPath).Content().Put(ctx, content, nil)
		if err != nil {
			return fmt.Errorf("uploading file: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			result := map[string]interface{}{
				"id":     deref(uploaded.GetId()),
				"name":   deref(uploaded.GetName()),
				"webUrl": deref(uploaded.GetWebUrl()),
				"size":   info.Size(),
			}
			return output.JSON(result)
		default:
			output.Success(fmt.Sprintf("Uploaded %s → %s (%s)", fileFlag, pathFlag, humanFileSize(info.Size())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  Registration
// ──────────────────────────────────────────────

func init() {
	// onedrive ls
	onedriveLsCmd.Flags().String("path", "", "OneDrive path (e.g., /Documents)")
	onedriveLsCmd.Flags().String("item-id", "", "Drive item ID")
	onedriveCmd.AddCommand(onedriveLsCmd)

	// onedrive get
	onedriveGetCmd.Flags().String("path", "", "OneDrive file path")
	onedriveGetCmd.Flags().String("item-id", "", "Drive item ID")
	onedriveGetCmd.Flags().String("output", "", "Local output file path (required)")
	onedriveGetCmd.Flags().Bool("force", false, "Overwrite existing local file")
	onedriveCmd.AddCommand(onedriveGetCmd)

	// onedrive upload
	onedriveUploadCmd.Flags().String("file", "", "Local file to upload (required)")
	onedriveUploadCmd.Flags().String("path", "", "OneDrive destination path (required)")
	onedriveUploadCmd.Flags().Bool("force", false, "Overwrite existing OneDrive file")
	onedriveCmd.AddCommand(onedriveUploadCmd)
}

