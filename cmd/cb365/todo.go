package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/nz365guy/cb365/internal/auth"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/graph"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Helpers
// ──────────────────────────────────────────────

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr(s string) *string { return &s }

func newGraphClient() (*msgraphsdkgo.GraphServiceClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	profileName := flagProfile
	if profileName == "" {
		profileName = cfg.ActiveProfile
	}
	if profileName == "" {
		return nil, fmt.Errorf("no active profile — run 'cb365 auth login' first")
	}

	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	cache, err := auth.LoadToken(profileName)
	if err != nil {
		return nil, fmt.Errorf("loading token: %w", err)
	}

	ipv4Only := auth.ShouldUseIPv4(cfg)

	info, err := auth.DecodeTokenInfo(cache.AccessToken)
	if err != nil || info.IsExpired {
		// Auto-refresh for app-only profiles
		if profile.AuthMode == config.AuthModeAppOnly && cache.ClientSecret != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if flagVerbose {
				output.Info("Token expired — refreshing via client credentials...")
			}

			token, refreshErr := auth.RefreshAppOnly(ctx, profile, cache, ipv4Only)
			if refreshErr != nil {
				return nil, fmt.Errorf("auto-refresh failed: %w", refreshErr)
			}

			cache.AccessToken = token.Token
			cache.ExpiresAt = token.ExpiresOn.Format(time.RFC3339)
			if storeErr := auth.StoreToken(profileName, cache); storeErr != nil {
				return nil, fmt.Errorf("storing refreshed token: %w", storeErr)
			}

			return graph.NewGraphClient(cache.AccessToken, token.ExpiresOn, ipv4Only)
		}

		return nil, fmt.Errorf("token expired — run 'cb365 auth login' to re-authenticate")
	}

	expiresAt, err := time.Parse(time.RFC3339, info.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parsing token expiry: %w", err)
	}

	return graph.NewGraphClient(cache.AccessToken, expiresAt, ipv4Only)
}

// resolveListID resolves a list name or ID to a Graph list ID.
func resolveListID(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, nameOrID string) (string, error) {
	// If it looks like a GUID or long Graph ID, use directly
	if len(nameOrID) == 36 && strings.Count(nameOrID, "-") == 4 {
		return nameOrID, nil
	}
	if len(nameOrID) > 36 && !strings.Contains(nameOrID, " ") {
		return nameOrID, nil
	}

	result, err := client.Me().Todo().Lists().Get(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("listing task lists for name resolution: %w", err)
	}

	target := strings.ToLower(nameOrID)
	for _, list := range result.GetValue() {
		if strings.ToLower(deref(list.GetDisplayName())) == target {
			return deref(list.GetId()), nil
		}
	}

	return "", fmt.Errorf("task list %q not found", nameOrID)
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var todoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Microsoft To Do — lists and tasks",
}

var todoListsCmd = &cobra.Command{
	Use:   "lists",
	Short: "Manage To Do task lists",
}

var todoTasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Manage tasks within a To Do list",
}

// ══════════════════════════════════════════════
//  LIST COMMANDS
// ══════════════════════════════════════════════

// ── todo lists list ──

var todoListsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all task lists",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Me().Todo().Lists().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching task lists: %w", err)
		}

		lists := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(lists))
			for _, l := range lists {
				item := map[string]interface{}{
					"id":           deref(l.GetId()),
					"display_name": deref(l.GetDisplayName()),
				}
				if l.GetIsOwner() != nil {
					item["is_owner"] = *l.GetIsOwner()
				}
				if l.GetIsShared() != nil {
					item["is_shared"] = *l.GetIsShared()
				}
				if l.GetWellknownListName() != nil {
					item["wellknown"] = l.GetWellknownListName().String()
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			var rows [][]string
			for _, l := range lists {
				rows = append(rows, []string{deref(l.GetId()), deref(l.GetDisplayName())})
			}
			output.Plain(rows)
		default:
			headers := []string{"NAME", "SHARED", "OWNER", "ID"}
			var rows [][]string
			for _, l := range lists {
				shared := "No"
				if l.GetIsShared() != nil && *l.GetIsShared() {
					shared = "Yes"
				}
				owner := "No"
				if l.GetIsOwner() != nil && *l.GetIsOwner() {
					owner = "Yes"
				}
				rows = append(rows, []string{deref(l.GetDisplayName()), shared, owner, deref(l.GetId())})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ── todo lists create ──

var todoListsCreateName string

var todoListsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoListsCreateName == "" {
			return fmt.Errorf("--name is required")
		}

		format := output.Resolve(flagJSON, flagPlain)

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create task list %q", todoListsCreateName))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "create_list", "display_name": todoListsCreateName})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		body := models.NewTodoTaskList()
		body.SetDisplayName(ptr(todoListsCreateName))

		result, err := client.Me().Todo().Lists().Post(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("creating task list: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{"id": deref(result.GetId()), "display_name": deref(result.GetDisplayName())})
		default:
			output.Success(fmt.Sprintf("Created list %q (ID: %s)", deref(result.GetDisplayName()), deref(result.GetId())))
		}
		return nil
	},
}

// ── todo lists update ──

var (
	todoListsUpdateList string
	todoListsUpdateName string
)

var todoListsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rename a task list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoListsUpdateList == "" || todoListsUpdateName == "" {
			return fmt.Errorf("--list and --name are required")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would rename list %q to %q", todoListsUpdateList, todoListsUpdateName))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "update_list", "list": todoListsUpdateList, "new_name": todoListsUpdateName})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoListsUpdateList)
		if err != nil {
			return err
		}

		body := models.NewTodoTaskList()
		body.SetDisplayName(ptr(todoListsUpdateName))

		result, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Patch(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("updating task list: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{"id": deref(result.GetId()), "display_name": deref(result.GetDisplayName())})
		default:
			output.Success(fmt.Sprintf("Renamed list to %q", deref(result.GetDisplayName())))
		}
		return nil
	},
}

// ── todo lists delete ──

var (
	todoListsDeleteList  string
	todoListsDeleteForce bool
)

var todoListsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a task list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoListsDeleteList == "" {
			return fmt.Errorf("--list is required")
		}
		if !todoListsDeleteForce {
			return fmt.Errorf("destructive operation — pass --force to confirm deletion")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would delete list %q", todoListsDeleteList))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "delete_list", "list": todoListsDeleteList})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoListsDeleteList)
		if err != nil {
			return err
		}

		if err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Delete(ctx, nil); err != nil {
			return fmt.Errorf("deleting task list: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]string{"deleted": listID, "status": "ok"})
		default:
			output.Success(fmt.Sprintf("Deleted list %q", todoListsDeleteList))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  TASK COMMANDS
// ══════════════════════════════════════════════

// ── todo tasks list ──

var todoTasksListList string

var todoTasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks in a To Do list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksListList == "" {
			return fmt.Errorf("--list is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksListList)
		if err != nil {
			return err
		}

		// Fetch tasks (first page — pagination TODO for very large lists)
		result, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching tasks: %w", err)
		}

		tasks := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(tasks))
			for _, t := range tasks {
				item := map[string]interface{}{
					"id":     deref(t.GetId()),
					"title":  deref(t.GetTitle()),
					"status": t.GetStatus().String(),
				}
				if t.GetBody() != nil && deref(t.GetBody().GetContent()) != "" {
					item["body"] = deref(t.GetBody().GetContent())
				}
				if t.GetDueDateTime() != nil {
					item["due_date"] = deref(t.GetDueDateTime().GetDateTime())
				}
				if t.GetImportance() != nil {
					item["importance"] = t.GetImportance().String()
				}
				if t.GetCreatedDateTime() != nil {
					item["created_at"] = t.GetCreatedDateTime().Format(time.RFC3339)
				}
				if t.GetLastModifiedDateTime() != nil {
					item["modified_at"] = t.GetLastModifiedDateTime().Format(time.RFC3339)
				}
				if t.GetCompletedDateTime() != nil {
					item["completed_at"] = deref(t.GetCompletedDateTime().GetDateTime())
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			var rows [][]string
			for _, t := range tasks {
				due := ""
				if t.GetDueDateTime() != nil {
					due = deref(t.GetDueDateTime().GetDateTime())
				}
				rows = append(rows, []string{deref(t.GetId()), deref(t.GetTitle()), t.GetStatus().String(), due})
			}
			output.Plain(rows)
		default:
			headers := []string{"TITLE", "STATUS", "DUE", "ID"}
			var rows [][]string
			for _, t := range tasks {
				status := "○ " + t.GetStatus().String()
				if t.GetStatus().String() == "completed" {
					status = "✓ completed"
				}
				due := ""
				if t.GetDueDateTime() != nil {
					due = deref(t.GetDueDateTime().GetDateTime())
					if len(due) > 10 {
						due = due[:10]
					}
				}
				rows = append(rows, []string{deref(t.GetTitle()), status, due, deref(t.GetId())})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ── todo tasks get ──

var (
	todoTasksGetList string
	todoTasksGetTask string
)

var todoTasksGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksGetList == "" || todoTasksGetTask == "" {
			return fmt.Errorf("--list and --task are required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksGetList)
		if err != nil {
			return err
		}

		task, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(todoTasksGetTask).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching task: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id":     deref(task.GetId()),
				"title":  deref(task.GetTitle()),
				"status": task.GetStatus().String(),
			}
			if task.GetBody() != nil {
				item["body"] = deref(task.GetBody().GetContent())
				item["body_type"] = task.GetBody().GetContentType().String()
			}
			if task.GetDueDateTime() != nil {
				item["due_date"] = deref(task.GetDueDateTime().GetDateTime())
				item["due_timezone"] = deref(task.GetDueDateTime().GetTimeZone())
			}
			if task.GetImportance() != nil {
				item["importance"] = task.GetImportance().String()
			}
			if task.GetCreatedDateTime() != nil {
				item["created_at"] = task.GetCreatedDateTime().Format(time.RFC3339)
			}
			return output.JSON(item)
		default:
			fmt.Printf("Title:     %s\n", deref(task.GetTitle()))
			fmt.Printf("Status:    %s\n", task.GetStatus().String())
			if task.GetDueDateTime() != nil {
				fmt.Printf("Due:       %s\n", deref(task.GetDueDateTime().GetDateTime()))
			}
			if task.GetBody() != nil && deref(task.GetBody().GetContent()) != "" {
				fmt.Printf("Body:      %s\n", deref(task.GetBody().GetContent()))
			}
			fmt.Printf("ID:        %s\n", deref(task.GetId()))
		}
		return nil
	},
}

// ── todo tasks create ──

var (
	todoTasksCreateList  string
	todoTasksCreateTitle string
	todoTasksCreateBody  string
	todoTasksCreateDue   string
)

var todoTasksCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksCreateList == "" || todoTasksCreateTitle == "" {
			return fmt.Errorf("--list and --title are required")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create task %q in list %q", todoTasksCreateTitle, todoTasksCreateList))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "create_task", "list": todoTasksCreateList, "title": todoTasksCreateTitle})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksCreateList)
		if err != nil {
			return err
		}

		body := models.NewTodoTask()
		body.SetTitle(ptr(todoTasksCreateTitle))

		if todoTasksCreateBody != "" {
			itemBody := models.NewItemBody()
			itemBody.SetContent(ptr(todoTasksCreateBody))
			contentType := models.TEXT_BODYTYPE
			itemBody.SetContentType(&contentType)
			body.SetBody(itemBody)
		}

		if todoTasksCreateDue != "" {
			dueDate := models.NewDateTimeTimeZone()
			dueDate.SetDateTime(ptr(todoTasksCreateDue + "T00:00:00"))
			dueDate.SetTimeZone(ptr("Pacific/Auckland"))
			body.SetDueDateTime(dueDate)
		}

		result, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().Post(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("creating task: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{"id": deref(result.GetId()), "title": deref(result.GetTitle()), "status": result.GetStatus().String()})
		default:
			output.Success(fmt.Sprintf("Created task %q (ID: %s)", deref(result.GetTitle()), deref(result.GetId())))
		}
		return nil
	},
}

// ── todo tasks update ──

var (
	todoTasksUpdateList   string
	todoTasksUpdateTask   string
	todoTasksUpdateTitle  string
	todoTasksUpdateBody   string
	todoTasksUpdateDue    string
	todoTasksUpdateStatus string
)

var todoTasksUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksUpdateList == "" || todoTasksUpdateTask == "" {
			return fmt.Errorf("--list and --task are required")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would update task %s in list %q", todoTasksUpdateTask, todoTasksUpdateList))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "update_task", "list": todoTasksUpdateList, "task": todoTasksUpdateTask})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksUpdateList)
		if err != nil {
			return err
		}

		body := models.NewTodoTask()
		hasUpdate := false

		if todoTasksUpdateTitle != "" {
			body.SetTitle(ptr(todoTasksUpdateTitle))
			hasUpdate = true
		}
		if todoTasksUpdateBody != "" {
			itemBody := models.NewItemBody()
			itemBody.SetContent(ptr(todoTasksUpdateBody))
			ct := models.TEXT_BODYTYPE
			itemBody.SetContentType(&ct)
			body.SetBody(itemBody)
			hasUpdate = true
		}
		if todoTasksUpdateDue != "" {
			dueDate := models.NewDateTimeTimeZone()
			dueDate.SetDateTime(ptr(todoTasksUpdateDue + "T00:00:00"))
			dueDate.SetTimeZone(ptr("Pacific/Auckland"))
			body.SetDueDateTime(dueDate)
			hasUpdate = true
		}
		if todoTasksUpdateStatus != "" {
			switch strings.ToLower(todoTasksUpdateStatus) {
			case "notstarted", "not_started":
				s := models.NOTSTARTED_TASKSTATUS
				body.SetStatus(&s)
			case "inprogress", "in_progress":
				s := models.INPROGRESS_TASKSTATUS
				body.SetStatus(&s)
			case "completed":
				s := models.COMPLETED_TASKSTATUS
				body.SetStatus(&s)
			case "waitingonothers", "waiting":
				s := models.WAITINGONOTHERS_TASKSTATUS
				body.SetStatus(&s)
			case "deferred":
				s := models.DEFERRED_TASKSTATUS
				body.SetStatus(&s)
			default:
				return fmt.Errorf("invalid status %q — use notStarted, inProgress, completed, waitingOnOthers, or deferred", todoTasksUpdateStatus)
			}
			hasUpdate = true
		}

		if !hasUpdate {
			return fmt.Errorf("nothing to update — specify --title, --body, --due, or --status")
		}

		result, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(todoTasksUpdateTask).Patch(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("updating task: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{"id": deref(result.GetId()), "title": deref(result.GetTitle()), "status": result.GetStatus().String()})
		default:
			output.Success(fmt.Sprintf("Updated task %q", deref(result.GetTitle())))
		}
		return nil
	},
}

// ── todo tasks complete ──

var (
	todoTasksCompleteList string
	todoTasksCompleteTask string
)

var todoTasksCompleteCmd = &cobra.Command{
	Use:   "complete",
	Short: "Mark a task as completed",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksCompleteList == "" || todoTasksCompleteTask == "" {
			return fmt.Errorf("--list and --task are required")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would complete task %s", todoTasksCompleteTask))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "complete_task", "task": todoTasksCompleteTask})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksCompleteList)
		if err != nil {
			return err
		}

		body := models.NewTodoTask()
		s := models.COMPLETED_TASKSTATUS
		body.SetStatus(&s)

		result, err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(todoTasksCompleteTask).Patch(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("completing task: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{"id": deref(result.GetId()), "title": deref(result.GetTitle()), "status": "completed"})
		default:
			output.Success(fmt.Sprintf("Completed task %q", deref(result.GetTitle())))
		}
		return nil
	},
}

// ── todo tasks delete ──

var (
	todoTasksDeleteList  string
	todoTasksDeleteTask  string
	todoTasksDeleteForce bool
)

var todoTasksDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if todoTasksDeleteList == "" || todoTasksDeleteTask == "" {
			return fmt.Errorf("--list and --task are required")
		}
		if !todoTasksDeleteForce {
			return fmt.Errorf("destructive operation — pass --force to confirm deletion")
		}

		format := output.Resolve(flagJSON, flagPlain)
		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would delete task %s", todoTasksDeleteTask))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{"dry_run": true, "action": "delete_task", "task": todoTasksDeleteTask})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		listID, err := resolveListID(ctx, client, todoTasksDeleteList)
		if err != nil {
			return err
		}

		if err := client.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(todoTasksDeleteTask).Delete(ctx, nil); err != nil {
			return fmt.Errorf("deleting task: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]string{"deleted": todoTasksDeleteTask, "status": "ok"})
		default:
			output.Success("Deleted task")
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  Wire up commands + flags
// ══════════════════════════════════════════════

func init() {
	// Lists subcommands
	todoListsCreateCmd.Flags().StringVar(&todoListsCreateName, "name", "", "Name for the new list")
	todoListsUpdateCmd.Flags().StringVar(&todoListsUpdateList, "list", "", "List name or ID")
	todoListsUpdateCmd.Flags().StringVar(&todoListsUpdateName, "name", "", "New name for the list")
	todoListsDeleteCmd.Flags().StringVar(&todoListsDeleteList, "list", "", "List name or ID")
	todoListsDeleteCmd.Flags().BoolVar(&todoListsDeleteForce, "force", false, "Confirm destructive operation")

	todoListsCmd.AddCommand(todoListsListCmd)
	todoListsCmd.AddCommand(todoListsCreateCmd)
	todoListsCmd.AddCommand(todoListsUpdateCmd)
	todoListsCmd.AddCommand(todoListsDeleteCmd)

	// Tasks subcommands
	todoTasksListCmd.Flags().StringVar(&todoTasksListList, "list", "", "List name or ID")
	todoTasksGetCmd.Flags().StringVar(&todoTasksGetList, "list", "", "List name or ID")
	todoTasksGetCmd.Flags().StringVar(&todoTasksGetTask, "task", "", "Task ID")
	todoTasksCreateCmd.Flags().StringVar(&todoTasksCreateList, "list", "", "List name or ID")
	todoTasksCreateCmd.Flags().StringVar(&todoTasksCreateTitle, "title", "", "Task title")
	todoTasksCreateCmd.Flags().StringVar(&todoTasksCreateBody, "body", "", "Task body/description")
	todoTasksCreateCmd.Flags().StringVar(&todoTasksCreateDue, "due", "", "Due date (YYYY-MM-DD)")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateList, "list", "", "List name or ID")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateTask, "task", "", "Task ID")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateTitle, "title", "", "New title")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateBody, "body", "", "New body")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateDue, "due", "", "New due date (YYYY-MM-DD)")
	todoTasksUpdateCmd.Flags().StringVar(&todoTasksUpdateStatus, "status", "", "Status: notStarted, inProgress, completed, waitingOnOthers, deferred")
	todoTasksCompleteCmd.Flags().StringVar(&todoTasksCompleteList, "list", "", "List name or ID")
	todoTasksCompleteCmd.Flags().StringVar(&todoTasksCompleteTask, "task", "", "Task ID")
	todoTasksDeleteCmd.Flags().StringVar(&todoTasksDeleteList, "list", "", "List name or ID")
	todoTasksDeleteCmd.Flags().StringVar(&todoTasksDeleteTask, "task", "", "Task ID")
	todoTasksDeleteCmd.Flags().BoolVar(&todoTasksDeleteForce, "force", false, "Confirm destructive operation")

	todoTasksCmd.AddCommand(todoTasksListCmd)
	todoTasksCmd.AddCommand(todoTasksGetCmd)
	todoTasksCmd.AddCommand(todoTasksCreateCmd)
	todoTasksCmd.AddCommand(todoTasksUpdateCmd)
	todoTasksCmd.AddCommand(todoTasksCompleteCmd)
	todoTasksCmd.AddCommand(todoTasksDeleteCmd)

	// Parent
	todoCmd.AddCommand(todoListsCmd)
	todoCmd.AddCommand(todoTasksCmd)
}
