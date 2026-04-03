package main

import (
	"context"
	"fmt"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	sitesPkg "github.com/microsoftgraph/msgraph-sdk-go/sites"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Loop — pages via SharePoint Site Pages API
//
//  Microsoft Loop stores pages as SharePoint site
//  pages. The Graph API exposes them via:
//    GET /sites/{siteId}/pages
//    GET /sites/{siteId}/pages/{pageId}
//    POST /sites/{siteId}/pages
//
//  The "workspace" in Loop maps to a SharePoint
//  site. Use `cb365 sharepoint sites list` to
//  find the site ID for a Loop workspace.
// ──────────────────────────────────────────────

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Microsoft Loop — pages and workspaces",
	Long: `Microsoft Loop operations via the SharePoint Site Pages API.

Loop workspaces map to SharePoint sites. Use 'cb365 sharepoint sites list'
to find the site ID for a Loop workspace, then use it as --workspace here.`,
}

var loopPagesCmd = &cobra.Command{
	Use:   "pages",
	Short: "Manage Loop pages",
}

// ──────────────────────────────────────────────
//  loop pages list
// ──────────────────────────────────────────────

var loopPagesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pages in a Loop workspace",
	Long: `List pages in a Loop workspace (SharePoint site).

Examples:
  cb365 loop pages list --workspace SITE_ID
  cb365 loop pages list --workspace SITE_ID --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		if wsFlag == "" {
			return fmt.Errorf("--workspace is required (SharePoint site ID for the Loop workspace)")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Sites().BySiteId(wsFlag).Pages().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing pages: %w", err)
		}

		pages := result.GetValue()

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(pages))
			for _, p := range pages {
				item := map[string]interface{}{
					"id":    deref(p.GetId()),
					"title": deref(p.GetTitle()),
				}
				if p.GetCreatedDateTime() != nil {
					item["createdAt"] = p.GetCreatedDateTime().Format(time.RFC3339)
				}
				if p.GetLastModifiedDateTime() != nil {
					item["lastModified"] = p.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if p.GetPageLayout() != nil {
					item["pageLayout"] = p.GetPageLayout().String()
				}
				if p.GetWebUrl() != nil {
					item["webUrl"] = deref(p.GetWebUrl())
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			rows := make([][]string, 0, len(pages))
			for _, p := range pages {
				rows = append(rows, []string{deref(p.GetId()), deref(p.GetTitle())})
			}
			output.Plain(rows)
		default:
			headers := []string{"ID", "TITLE", "LAYOUT", "LAST MODIFIED"}
			rows := make([][]string, 0, len(pages))
			for _, p := range pages {
				layout := ""
				if p.GetPageLayout() != nil {
					layout = p.GetPageLayout().String()
				}
				lastMod := ""
				if p.GetLastModifiedDateTime() != nil {
					lastMod = p.GetLastModifiedDateTime().Format("2006-01-02 15:04")
				}
				rows = append(rows, []string{deref(p.GetId()), deref(p.GetTitle()), layout, lastMod})
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
	Short: "Get a Loop page by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		pageFlag, _ := cmd.Flags().GetString("page")

		if wsFlag == "" {
			return fmt.Errorf("--workspace is required")
		}
		if pageFlag == "" {
			return fmt.Errorf("--page is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get the page as a site page with expanded canvas layout
		config := &sitesPkg.ItemPagesBaseSitePageItemRequestBuilderGetRequestConfiguration{
			QueryParameters: &sitesPkg.ItemPagesBaseSitePageItemRequestBuilderGetQueryParameters{
				Expand: []string{"canvasLayout"},
			},
		}

		page, err := client.Sites().BySiteId(wsFlag).Pages().ByBaseSitePageId(pageFlag).Get(ctx, config)
		if err != nil {
			return fmt.Errorf("getting page: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id":    deref(page.GetId()),
				"title": deref(page.GetTitle()),
			}
			if page.GetCreatedDateTime() != nil {
				item["createdAt"] = page.GetCreatedDateTime().Format(time.RFC3339)
			}
			if page.GetLastModifiedDateTime() != nil {
				item["lastModified"] = page.GetLastModifiedDateTime().Format(time.RFC3339)
			}
			if page.GetPageLayout() != nil {
				item["pageLayout"] = page.GetPageLayout().String()
			}
			if page.GetWebUrl() != nil {
				item["webUrl"] = deref(page.GetWebUrl())
			}
			if page.GetPublishingState() != nil {
				pub := page.GetPublishingState()
				state := map[string]interface{}{}
				if pub.GetLevel() != nil {
					if pub.GetLevel() != nil { state["level"] = *pub.GetLevel() }
				}
				item["publishingState"] = state
			}
			return output.JSON(item)
		default:
			output.Info(fmt.Sprintf("Page:  %s", deref(page.GetTitle())))
			output.Info(fmt.Sprintf("ID:    %s", deref(page.GetId())))
			if page.GetWebUrl() != nil {
				output.Info(fmt.Sprintf("URL:   %s", deref(page.GetWebUrl())))
			}
			if page.GetPageLayout() != nil {
				output.Info(fmt.Sprintf("Layout: %s", page.GetPageLayout().String()))
			}
			if page.GetLastModifiedDateTime() != nil {
				output.Info(fmt.Sprintf("Modified: %s", page.GetLastModifiedDateTime().Format("2006-01-02 15:04")))
			}
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  loop pages create
// ──────────────────────────────────────────────

var loopPagesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Loop page",
	Long: `Create a new page in a Loop workspace.

Safety: --dry-run to preview. Read-only until Loop write safety rules are validated.

Examples:
  cb365 loop pages create --workspace SITE_ID --title "Meeting Notes"
  cb365 loop pages create --workspace SITE_ID --title "Sprint Plan" --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsFlag, _ := cmd.Flags().GetString("workspace")
		titleFlag, _ := cmd.Flags().GetString("title")

		if wsFlag == "" {
			return fmt.Errorf("--workspace is required")
		}
		if titleFlag == "" {
			return fmt.Errorf("--title is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would create page %q in workspace %s", titleFlag, wsFlag))
			return nil
		}

		page := models.NewSitePage()
		page.SetTitle(&titleFlag)
		// Set page layout to article (standard page type)
		layout := models.ARTICLE_PAGELAYOUTTYPE
		page.SetPageLayout(&layout)
		// Set the OData type for proper serialisation
		odataType := "#microsoft.graph.sitePage"
		page.SetOdataType(&odataType)

		created, err := client.Sites().BySiteId(wsFlag).Pages().Post(ctx, page, nil)
		if err != nil {
			return fmt.Errorf("creating page: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id":    deref(created.GetId()),
				"title": deref(created.GetTitle()),
			}
			if created.GetWebUrl() != nil {
				item["webUrl"] = deref(created.GetWebUrl())
			}
			return output.JSON(item)
		default:
			output.Success(fmt.Sprintf("Created page %q (id: %s)", titleFlag, deref(created.GetId())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  Registration
// ──────────────────────────────────────────────

func init() {
	// loop pages list
	loopPagesListCmd.Flags().String("workspace", "", "Loop workspace ID (SharePoint site ID)")
	loopPagesCmd.AddCommand(loopPagesListCmd)

	// loop pages get
	loopPagesGetCmd.Flags().String("workspace", "", "Loop workspace ID (SharePoint site ID)")
	loopPagesGetCmd.Flags().String("page", "", "Page ID")
	loopPagesCmd.AddCommand(loopPagesGetCmd)

	// loop pages create
	loopPagesCreateCmd.Flags().String("workspace", "", "Loop workspace ID (SharePoint site ID)")
	loopPagesCreateCmd.Flags().String("title", "", "Page title")
	loopPagesCmd.AddCommand(loopPagesCreateCmd)

	// Wire up
	loopCmd.AddCommand(loopPagesCmd)
}

