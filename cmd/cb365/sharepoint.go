package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/sites"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  SharePoint helpers
// ──────────────────────────────────────────────

// formatSiteURL builds a human-readable site URL from hostname and server-relative path.
func formatSiteURL(webURL string) string {
	if webURL != "" {
		return webURL
	}
	return "(unknown)"
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var sharepointCmd = &cobra.Command{
	Use:     "sharepoint",
	Aliases: []string{"sp"},
	Short:   "SharePoint — sites, lists, and list items",
}

var sharepointSitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "Manage SharePoint sites",
}

var sharepointListsCmd = &cobra.Command{
	Use:   "lists",
	Short: "Manage SharePoint lists",
}

// ──────────────────────────────────────────────
//  sharepoint sites list
// ──────────────────────────────────────────────

var sharepointSitesListCmd = &cobra.Command{
	Use:   "list",
	Short: "Search and list SharePoint sites",
	Long: `Search SharePoint sites by keyword. Without --search, lists root sites.

Examples:
  cb365 sharepoint sites list --search "Marketing"
  cb365 sharepoint sites list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		searchFlag, _ := cmd.Flags().GetString("search")

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		type siteInfo struct {
			ID          string
			DisplayName string
			WebURL      string
			Description string
		}

		var siteList []siteInfo

		if searchFlag != "" {
			// Use the search endpoint: GET /sites?search=query
			config := &sites.SitesRequestBuilderGetRequestConfiguration{
				QueryParameters: &sites.SitesRequestBuilderGetQueryParameters{
					Search: &searchFlag,
				},
			}

			result, err := client.Sites().Get(ctx, config)
			if err != nil {
				return fmt.Errorf("searching sites: %w", err)
			}

			for _, s := range result.GetValue() {
				siteList = append(siteList, siteInfo{
					ID:          deref(s.GetId()),
					DisplayName: deref(s.GetDisplayName()),
					WebURL:      deref(s.GetWebUrl()),
					Description: deref(s.GetDescription()),
				})
			}
		} else {
			// List root site and sub-sites
			// GET /sites/root
			root, err := client.Sites().BySiteId("root").Get(ctx, nil)
			if err != nil {
				return fmt.Errorf("getting root site: %w", err)
			}
			siteList = append(siteList, siteInfo{
				ID:          deref(root.GetId()),
				DisplayName: deref(root.GetDisplayName()),
				WebURL:      deref(root.GetWebUrl()),
				Description: deref(root.GetDescription()),
			})

			// Also try to list all sites (requires Sites.Read.All)
			allConfig := &sites.SitesRequestBuilderGetRequestConfiguration{
				QueryParameters: &sites.SitesRequestBuilderGetQueryParameters{
					Search: ptr("*"),
				},
			}
			allResult, err := client.Sites().Get(ctx, allConfig)
			if err == nil {
				for _, s := range allResult.GetValue() {
					id := deref(s.GetId())
					// Skip root (already added)
					if id == deref(root.GetId()) {
						continue
					}
					siteList = append(siteList, siteInfo{
						ID:          id,
						DisplayName: deref(s.GetDisplayName()),
						WebURL:      deref(s.GetWebUrl()),
						Description: deref(s.GetDescription()),
					})
				}
			}
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(siteList))
			for _, s := range siteList {
				items = append(items, map[string]interface{}{
					"id":          s.ID,
					"displayName": s.DisplayName,
					"webUrl":      s.WebURL,
					"description": s.Description,
				})
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(siteList))
			for _, s := range siteList {
				rows = append(rows, []string{s.ID, s.DisplayName, s.WebURL})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "NAME", "URL"}
			rows := make([][]string, 0, len(siteList))
			for _, s := range siteList {
				name := s.DisplayName
				if name == "" {
					name = "(root)"
				}
				rows = append(rows, []string{s.ID, name, formatSiteURL(s.WebURL)})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint sites get
// ──────────────────────────────────────────────

var sharepointSitesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get details of a SharePoint site",
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		site, err := client.Sites().BySiteId(siteFlag).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting site: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id":          deref(site.GetId()),
				"displayName": deref(site.GetDisplayName()),
				"webUrl":      deref(site.GetWebUrl()),
				"description": deref(site.GetDescription()),
			}
			if site.GetCreatedDateTime() != nil {
				item["createdAt"] = site.GetCreatedDateTime().Format(time.RFC3339)
			}
			return output.JSON(item)
		default:
			output.Info(fmt.Sprintf("Site: %s", deref(site.GetDisplayName())))
			output.Info(fmt.Sprintf("ID:   %s", deref(site.GetId())))
			output.Info(fmt.Sprintf("URL:  %s", deref(site.GetWebUrl())))
			if deref(site.GetDescription()) != "" {
				output.Info(fmt.Sprintf("Desc: %s", deref(site.GetDescription())))
			}
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint lists list
// ──────────────────────────────────────────────

var sharepointListsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List SharePoint lists in a site",
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Sites().BySiteId(siteFlag).Lists().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing lists: %w", err)
		}

		lists := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(lists))
			for _, l := range lists {
				item := map[string]interface{}{
					"id":          deref(l.GetId()),
					"displayName": deref(l.GetDisplayName()),
					"description": deref(l.GetDescription()),
					"webUrl":      deref(l.GetWebUrl()),
				}
				if l.GetList() != nil && l.GetList().GetTemplate() != nil {
					if l.GetList().GetTemplate() != nil { item["template"] = *l.GetList().GetTemplate() }
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(lists))
			for _, l := range lists {
				rows = append(rows, []string{deref(l.GetId()), deref(l.GetDisplayName())})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "NAME", "URL"}
			rows := make([][]string, 0, len(lists))
			for _, l := range lists {
				rows = append(rows, []string{deref(l.GetId()), deref(l.GetDisplayName()), deref(l.GetWebUrl())})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint lists items
// ──────────────────────────────────────────────

var sharepointListsItemsCmd = &cobra.Command{
	Use:   "items",
	Short: "Manage items in a SharePoint list",
}

var sharepointListsItemsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List items in a SharePoint list",
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		listFlag, _ := cmd.Flags().GetString("list")
		maxFlag, _ := cmd.Flags().GetInt("max")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if listFlag == "" {
			return fmt.Errorf("--list is required")
		}
		if maxFlag <= 0 {
			maxFlag = 50
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		top := int32(maxFlag)
		expand := []string{"fields"}
		config := &sites.ItemListsItemItemsRequestBuilderGetRequestConfiguration{
			QueryParameters: &sites.ItemListsItemItemsRequestBuilderGetQueryParameters{
				Top:    &top,
				Expand: expand,
			},
		}

		result, err := client.Sites().BySiteId(siteFlag).Lists().ByListId(listFlag).Items().Get(ctx, config)
		if err != nil {
			return fmt.Errorf("listing items: %w", err)
		}

		listItems := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(listItems))
			for _, li := range listItems {
				item := map[string]interface{}{
					"id":      deref(li.GetId()),
					"webUrl":  deref(li.GetWebUrl()),
				}
				if li.GetCreatedDateTime() != nil {
					item["createdAt"] = li.GetCreatedDateTime().Format(time.RFC3339)
				}
				if li.GetLastModifiedDateTime() != nil {
					item["lastModified"] = li.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				// Include expanded fields
				if li.GetFields() != nil {
					fields := li.GetFields().GetAdditionalData()
					if fields != nil {
						item["fields"] = fields
					}
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(listItems))
			for _, li := range listItems {
				rows = append(rows, []string{deref(li.GetId()), deref(li.GetWebUrl())})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "WEB URL", "LAST MODIFIED"}
			rows := make([][]string, 0, len(listItems))
			for _, li := range listItems {
				lastMod := ""
				if li.GetLastModifiedDateTime() != nil {
					lastMod = li.GetLastModifiedDateTime().Format("2006-01-02 15:04")
				}
				rows = append(rows, []string{deref(li.GetId()), deref(li.GetWebUrl()), lastMod})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}


// ──────────────────────────────────────────────
//  sharepoint lists items create
// ──────────────────────────────────────────────

var sharepointListsItemsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new item in a SharePoint list",
	Long: `Create a new item in a SharePoint list with field values.

Fields are specified as key=value pairs via --field flags.

Examples:
  cb365 sp lists items create --site SITE_ID --list LIST_ID --field Title="New Item" --field Status="Active"
  cb365 sp lists items create --site SITE_ID --list LIST_ID --field Title="Task" --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		listFlag, _ := cmd.Flags().GetString("list")
		fieldFlags, _ := cmd.Flags().GetStringSlice("field")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if listFlag == "" {
			return fmt.Errorf("--list is required")
		}
		if len(fieldFlags) == 0 {
			return fmt.Errorf("at least one --field is required (format: Key=Value)")
		}

		// Parse field key=value pairs
		fields := make(map[string]interface{})
		for _, f := range fieldFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid field format %q — use Key=Value", f)
			}
			fields[parts[0]] = parts[1]
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would create list item with %d fields in list %s", len(fields), listFlag))
			return nil
		}

		item := models.NewListItem()
		fieldSet := models.NewFieldValueSet()
		fieldSet.SetAdditionalData(fields)
		item.SetFields(fieldSet)

		created, err := client.Sites().BySiteId(siteFlag).Lists().ByListId(listFlag).Items().Post(ctx, item, nil)
		if err != nil {
			return fmt.Errorf("creating list item: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			result := map[string]interface{}{
				"id": deref(created.GetId()),
			}
			if created.GetFields() != nil {
				result["fields"] = created.GetFields().GetAdditionalData()
			}
			return output.JSON(result)
		default:
			output.Success(fmt.Sprintf("Created list item (id: %s)", deref(created.GetId())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint lists items update
// ──────────────────────────────────────────────

var sharepointListsItemsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update fields on a SharePoint list item",
	Long: `Update field values on an existing SharePoint list item.

Examples:
  cb365 sp lists items update --site SITE_ID --list LIST_ID --item ITEM_ID --field Status="Complete"
  cb365 sp lists items update --site SITE_ID --list LIST_ID --item ITEM_ID --field Title="Updated" --field Priority="High"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		listFlag, _ := cmd.Flags().GetString("list")
		itemFlag, _ := cmd.Flags().GetString("item")
		fieldFlags, _ := cmd.Flags().GetStringSlice("field")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if listFlag == "" {
			return fmt.Errorf("--list is required")
		}
		if itemFlag == "" {
			return fmt.Errorf("--item is required")
		}
		if len(fieldFlags) == 0 {
			return fmt.Errorf("at least one --field is required (format: Key=Value)")
		}

		fields := make(map[string]interface{})
		for _, f := range fieldFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid field format %q — use Key=Value", f)
			}
			fields[parts[0]] = parts[1]
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would update item %s with %d fields", itemFlag, len(fields)))
			return nil
		}

		fieldSet := models.NewFieldValueSet()
		fieldSet.SetAdditionalData(fields)

		updated, err := client.Sites().BySiteId(siteFlag).Lists().ByListId(listFlag).Items().ByListItemId(itemFlag).Fields().Patch(ctx, fieldSet, nil)
		if err != nil {
			return fmt.Errorf("updating list item: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			result := map[string]interface{}{
				"id":     itemFlag,
				"fields": updated.GetAdditionalData(),
			}
			return output.JSON(result)
		default:
			output.Success(fmt.Sprintf("Updated list item %s", itemFlag))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint lists items delete
// ──────────────────────────────────────────────

var sharepointListsItemsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a SharePoint list item",
	Long: `Delete an item from a SharePoint list. Requires --force.

Examples:
  cb365 sp lists items delete --site SITE_ID --list LIST_ID --item ITEM_ID --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		listFlag, _ := cmd.Flags().GetString("list")
		itemFlag, _ := cmd.Flags().GetString("item")
		forceFlag, _ := cmd.Flags().GetBool("force")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if listFlag == "" {
			return fmt.Errorf("--list is required")
		}
		if itemFlag == "" {
			return fmt.Errorf("--item is required")
		}
		if !forceFlag {
			return fmt.Errorf("deleting list items is destructive — pass --force to confirm")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would delete item %s from list %s", itemFlag, listFlag))
			return nil
		}

		err = client.Sites().BySiteId(siteFlag).Lists().ByListId(listFlag).Items().ByListItemId(itemFlag).Delete(ctx, nil)
		if err != nil {
			return fmt.Errorf("deleting list item: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":      itemFlag,
				"deleted": true,
			})
		default:
			output.Success(fmt.Sprintf("Deleted list item %s", itemFlag))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  sharepoint files parent + commands
// ──────────────────────────────────────────────

var sharepointFilesCmd = &cobra.Command{
	Use:   "files",
	Short: "Manage files in SharePoint document libraries",
}

var sharepointFilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List files in a site's document library",
	Long: `List files in a SharePoint site's default document library.

Examples:
  cb365 sp files list --site SITE_ID
  cb365 sp files list --site SITE_ID --path "/Shared Documents/Reports"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		pathFlag, _ := cmd.Flags().GetString("path")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get the site's default drive (document library)
		drive, err := client.Sites().BySiteId(siteFlag).Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting site drive: %w", err)
		}
		driveID := deref(drive.GetId())

		parentID := "root"
		if pathFlag != "" && pathFlag != "/" {
			cleanPath := strings.TrimPrefix(pathFlag, "/")
			parentID = fmt.Sprintf("root:/%s:", cleanPath)
		}

		result, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(parentID).Children().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing files: %w", err)
		}

		items := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			jsonItems := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				entry := map[string]interface{}{
					"id":       deref(item.GetId()),
					"name":     deref(item.GetName()),
					"isFolder": item.GetFolder() != nil,
					"webUrl":   deref(item.GetWebUrl()),
				}
				if item.GetSize() != nil {
					entry["size"] = *item.GetSize()
				}
				if item.GetLastModifiedDateTime() != nil {
					entry["lastModified"] = item.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				jsonItems = append(jsonItems, entry)
			}
			return output.JSON(jsonItems)
		default:
			headers := []string{"TYPE", "NAME", "SIZE", "LAST MODIFIED"}
			rows := make([][]string, 0, len(items))
			for _, item := range items {
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
				rows = append(rows, []string{typeStr, deref(item.GetName()), sizeStr, lastMod})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

var sharepointFilesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Download a file from a SharePoint document library",
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		pathFlag, _ := cmd.Flags().GetString("path")
		itemIDFlag, _ := cmd.Flags().GetString("item-id")
		outputFlag, _ := cmd.Flags().GetString("output")
		forceFlag, _ := cmd.Flags().GetBool("force")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if pathFlag == "" && itemIDFlag == "" {
			return fmt.Errorf("--path or --item-id is required")
		}
		if outputFlag == "" {
			return fmt.Errorf("--output is required")
		}

		// Safety: no overwrite without --force
		if !forceFlag {
			if _, statErr := os.Stat(outputFlag); statErr == nil {
				return fmt.Errorf("output file %q already exists — use --force to overwrite", outputFlag)
			}
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		drive, err := client.Sites().BySiteId(siteFlag).Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting site drive: %w", err)
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

		// Safe write via temp file
		dir := filepath.Dir(outputFlag)
		tmpFile, tmpErr := os.CreateTemp(dir, ".cb365-sp-*")
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
				"path": outputFlag,
				"size": len(content),
			})
		default:
			output.Success(fmt.Sprintf("Downloaded %s (%s)", outputFlag, humanFileSize(int64(len(content)))))
		}
		return nil
	},
}

var sharepointFilesUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a file to a SharePoint document library",
	Long: `Upload a local file to a SharePoint site's document library.

Safety: 4MB simple upload limit. --force required to overwrite.

Examples:
  cb365 sp files upload --site SITE_ID --file ./report.pdf --path "/Shared Documents/report.pdf"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		siteFlag, _ := cmd.Flags().GetString("site")
		fileFlag, _ := cmd.Flags().GetString("file")
		pathFlag, _ := cmd.Flags().GetString("path")
		forceFlag, _ := cmd.Flags().GetBool("force")

		if siteFlag == "" {
			return fmt.Errorf("--site is required")
		}
		if fileFlag == "" {
			return fmt.Errorf("--file is required (local file path)")
		}
		if pathFlag == "" {
			return fmt.Errorf("--path is required (SharePoint destination path)")
		}

		info, err := os.Stat(fileFlag)
		if err != nil {
			return fmt.Errorf("reading local file: %w", err)
		}

		const maxSimpleUpload = 4 * 1024 * 1024
		if info.Size() > maxSimpleUpload {
			return fmt.Errorf("file is %s — simple upload limit is 4MB", humanFileSize(info.Size()))
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

		drive, err := client.Sites().BySiteId(siteFlag).Drive().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting site drive: %w", err)
		}
		driveID := deref(drive.GetId())

		// Check if exists (unless --force)
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

		content, err := os.ReadFile(fileFlag) // #nosec G304
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		cleanPath := strings.TrimPrefix(pathFlag, "/")
		itemByPath := fmt.Sprintf("root:/%s:", cleanPath)

		uploaded, err := client.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemByPath).Content().Put(ctx, content, nil)
		if err != nil {
			return fmt.Errorf("uploading file: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":     deref(uploaded.GetId()),
				"name":   deref(uploaded.GetName()),
				"webUrl": deref(uploaded.GetWebUrl()),
				"size":   info.Size(),
			})
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
	// sharepoint sites list
	sharepointSitesListCmd.Flags().String("search", "", "Search keyword")
	sharepointSitesCmd.AddCommand(sharepointSitesListCmd)

	// sharepoint sites get
	sharepointSitesGetCmd.Flags().String("site", "", "Site ID (required)")
	sharepointSitesCmd.AddCommand(sharepointSitesGetCmd)

	// sharepoint lists list
	sharepointListsListCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsCmd.AddCommand(sharepointListsListCmd)

	// sharepoint lists items list
	sharepointListsItemsListCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsItemsListCmd.Flags().String("list", "", "List ID (required)")
	sharepointListsItemsListCmd.Flags().Int("max", 50, "Maximum items to return")
	sharepointListsItemsCmd.AddCommand(sharepointListsItemsListCmd)

	// sharepoint lists items create
	sharepointListsItemsCreateCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsItemsCreateCmd.Flags().String("list", "", "List ID (required)")
	sharepointListsItemsCreateCmd.Flags().StringSlice("field", nil, "Field value as Key=Value (repeatable)")
	sharepointListsItemsCmd.AddCommand(sharepointListsItemsCreateCmd)

	// sharepoint lists items update
	sharepointListsItemsUpdateCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsItemsUpdateCmd.Flags().String("list", "", "List ID (required)")
	sharepointListsItemsUpdateCmd.Flags().String("item", "", "Item ID (required)")
	sharepointListsItemsUpdateCmd.Flags().StringSlice("field", nil, "Field value as Key=Value (repeatable)")
	sharepointListsItemsCmd.AddCommand(sharepointListsItemsUpdateCmd)

	// sharepoint lists items delete
	sharepointListsItemsDeleteCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsItemsDeleteCmd.Flags().String("list", "", "List ID (required)")
	sharepointListsItemsDeleteCmd.Flags().String("item", "", "Item ID (required)")
	sharepointListsItemsDeleteCmd.Flags().Bool("force", false, "Confirm deletion (required)")
	sharepointListsItemsCmd.AddCommand(sharepointListsItemsDeleteCmd)

	sharepointListsCmd.AddCommand(sharepointListsItemsCmd)

	// sharepoint files list
	sharepointFilesListCmd.Flags().String("site", "", "Site ID (required)")
	sharepointFilesListCmd.Flags().String("path", "", "Folder path in document library")
	sharepointFilesCmd.AddCommand(sharepointFilesListCmd)

	// sharepoint files get
	sharepointFilesGetCmd.Flags().String("site", "", "Site ID (required)")
	sharepointFilesGetCmd.Flags().String("path", "", "File path in document library")
	sharepointFilesGetCmd.Flags().String("item-id", "", "Drive item ID")
	sharepointFilesGetCmd.Flags().String("output", "", "Local output file path (required)")
	sharepointFilesGetCmd.Flags().Bool("force", false, "Overwrite existing local file")
	sharepointFilesCmd.AddCommand(sharepointFilesGetCmd)

	// sharepoint files upload
	sharepointFilesUploadCmd.Flags().String("site", "", "Site ID (required)")
	sharepointFilesUploadCmd.Flags().String("file", "", "Local file to upload (required)")
	sharepointFilesUploadCmd.Flags().String("path", "", "SharePoint destination path (required)")
	sharepointFilesUploadCmd.Flags().Bool("force", false, "Overwrite existing file")
	sharepointFilesCmd.AddCommand(sharepointFilesUploadCmd)

	// Wire up
	sharepointCmd.AddCommand(sharepointSitesCmd)
	sharepointCmd.AddCommand(sharepointListsCmd)
	sharepointCmd.AddCommand(sharepointFilesCmd)
}

