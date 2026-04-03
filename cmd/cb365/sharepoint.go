package main

import (
	"context"
	"fmt"
	"time"

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

	// sharepoint lists items
	sharepointListsItemsCmd.Flags().String("site", "", "Site ID (required)")
	sharepointListsItemsCmd.Flags().String("list", "", "List ID (required)")
	sharepointListsItemsCmd.Flags().Int("max", 50, "Maximum items to return")
	sharepointListsCmd.AddCommand(sharepointListsItemsCmd)

	// Wire up
	sharepointCmd.AddCommand(sharepointSitesCmd)
	sharepointCmd.AddCommand(sharepointListsCmd)
}

