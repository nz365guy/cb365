package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	plannerPkg "github.com/microsoftgraph/msgraph-sdk-go/planner"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Planner helpers
// ──────────────────────────────────────────────

// getETag extracts the @odata.etag from an entity's additional data.
// Planner API requires If-Match headers on PATCH/DELETE operations.
func getETag(additionalData map[string]any) string {
	if v, ok := additionalData["@odata.etag"]; ok {
		if s, ok := v.(*string); ok && s != nil {
			return *s
		}
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// resolveUserID resolves an email/UPN or user ID to a Graph user ID.
// Graph accepts both GUID and UPN for /users/{id-or-userPrincipalName}.
func resolveUserID(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, emailOrID string) (string, error) {
	user, err := client.Users().ByUserId(emailOrID).Get(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("looking up user %q: %w", emailOrID, err)
	}
	return deref(user.GetId()), nil
}

// plannerPercentComplete maps progress string to Planner percent values.
// Planner uses: 0 = Not started, 50 = In progress, 100 = Complete
func plannerPercentComplete(progress string) (int32, error) {
	switch strings.ToLower(progress) {
	case "not-started", "notstarted", "0":
		return 0, nil
	case "in-progress", "inprogress", "50":
		return 50, nil
	case "complete", "completed", "done", "100":
		return 100, nil
	default:
		return 0, fmt.Errorf("invalid progress %q — use not-started, in-progress, or complete", progress)
	}
}

// ──────────────────────────────────────────────
//  Parent commands
// ──────────────────────────────────────────────

var plannerCmd = &cobra.Command{
	Use:   "planner",
	Short: "Microsoft Planner — plans, buckets, and tasks",
}

var plannerPlansCmd = &cobra.Command{
	Use:   "plans",
	Short: "Manage Planner plans",
}

var plannerBucketsCmd = &cobra.Command{
	Use:   "buckets",
	Short: "Manage Planner buckets",
}

var plannerTasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Manage Planner tasks",
}

// ══════════════════════════════════════════════
//  PLANS COMMANDS
// ══════════════════════════════════════════════

// ── planner plans list ──

var plannerPlansListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Planner plans assigned to you",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Me().Planner().Plans().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching planner plans: %w", err)
		}

		plans := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(plans))
			for _, p := range plans {
				item := map[string]interface{}{
					"id":    deref(p.GetId()),
					"title": deref(p.GetTitle()),
				}
				if p.GetCreatedDateTime() != nil {
					item["created"] = p.GetCreatedDateTime().Format(time.RFC3339)
				}
				if p.GetContainer() != nil {
					item["group_id"] = deref(p.GetContainer().GetContainerId())
				}
				if p.GetCreatedBy() != nil && p.GetCreatedBy().GetUser() != nil {
					item["created_by"] = deref(p.GetCreatedBy().GetUser().GetDisplayName())
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			var rows [][]string
			for _, p := range plans {
				rows = append(rows, []string{deref(p.GetId()), deref(p.GetTitle())})
			}
			output.Plain(rows)
		default:
			headers := []string{"TITLE", "GROUP ID", "ID"}
			var rows [][]string
			for _, p := range plans {
				groupID := ""
				if p.GetContainer() != nil {
					groupID = deref(p.GetContainer().GetContainerId())
				}
				rows = append(rows, []string{
					deref(p.GetTitle()),
					groupID,
					deref(p.GetId()),
				})
			}
			output.Table(headers, rows)
			output.Success(fmt.Sprintf("%d plan(s) found", len(plans)))
		}
		return nil
	},
}

// ── planner plans create ──

var (
	planCreateName    string
	planCreateGroupID string
)

var plannerPlansCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Planner plan in an M365 Group",
	RunE: func(cmd *cobra.Command, args []string) error {
		if planCreateName == "" {
			return fmt.Errorf("--name is required")
		}
		if planCreateGroupID == "" {
			return fmt.Errorf("--group-id is required — plans must be owned by an M365 Group")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create plan %q in group %s", planCreateName, planCreateGroupID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "create_plan",
					"name": planCreateName, "group_id": planCreateGroupID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		plan := models.NewPlannerPlan()
		plan.SetTitle(ptr(planCreateName))

		container := models.NewPlannerPlanContainer()
		container.SetContainerId(ptr(planCreateGroupID))
		containerType := models.GROUP_PLANNERCONTAINERTYPE
		container.SetTypeEscaped(&containerType)
		plan.SetContainer(container)

		created, err := client.Planner().Plans().Post(ctx, plan, nil)
		if err != nil {
			return fmt.Errorf("creating plan: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":       deref(created.GetId()),
				"title":    deref(created.GetTitle()),
				"group_id": planCreateGroupID,
			})
		default:
			output.Success(fmt.Sprintf("Created plan %q (ID: %s)", deref(created.GetTitle()), deref(created.GetId())))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  BUCKETS COMMANDS
// ══════════════════════════════════════════════

// ── planner buckets list ──

var bucketListPlanID string

var plannerBucketsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List buckets in a Planner plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		if bucketListPlanID == "" {
			return fmt.Errorf("--plan is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Planner().Plans().ByPlannerPlanId(bucketListPlanID).Buckets().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching buckets: %w", err)
		}

		buckets := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(buckets))
			for _, b := range buckets {
				items = append(items, map[string]interface{}{
					"id":      deref(b.GetId()),
					"name":    deref(b.GetName()),
					"plan_id": deref(b.GetPlanId()),
				})
			}
			return output.JSON(items)
		case output.FormatPlain:
			var rows [][]string
			for _, b := range buckets {
				rows = append(rows, []string{deref(b.GetId()), deref(b.GetName())})
			}
			output.Plain(rows)
		default:
			headers := []string{"NAME", "ID"}
			var rows [][]string
			for _, b := range buckets {
				rows = append(rows, []string{deref(b.GetName()), deref(b.GetId())})
			}
			output.Table(headers, rows)
			output.Success(fmt.Sprintf("%d bucket(s) found", len(buckets)))
		}
		return nil
	},
}

// ── planner buckets create ──

var (
	bucketCreatePlanID string
	bucketCreateName   string
)

var plannerBucketsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new bucket in a Planner plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		if bucketCreatePlanID == "" {
			return fmt.Errorf("--plan is required")
		}
		if bucketCreateName == "" {
			return fmt.Errorf("--name is required")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create bucket %q in plan %s", bucketCreateName, bucketCreatePlanID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "create_bucket",
					"name": bucketCreateName, "plan_id": bucketCreatePlanID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		bucket := models.NewPlannerBucket()
		bucket.SetName(ptr(bucketCreateName))
		bucket.SetPlanId(ptr(bucketCreatePlanID))

		created, err := client.Planner().Buckets().Post(ctx, bucket, nil)
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id":      deref(created.GetId()),
				"name":    deref(created.GetName()),
				"plan_id": deref(created.GetPlanId()),
			})
		default:
			output.Success(fmt.Sprintf("Created bucket %q (ID: %s)", deref(created.GetName()), deref(created.GetId())))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  TASKS COMMANDS
// ══════════════════════════════════════════════

// ── planner tasks list ──

var (
	taskListPlanID   string
	taskListBucketID string
)

var plannerTasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks in a Planner plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskListPlanID == "" {
			return fmt.Errorf("--plan is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Planner().Plans().ByPlannerPlanId(taskListPlanID).Tasks().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching tasks: %w", err)
		}

		tasks := result.GetValue()

		// Client-side filter by bucket if specified
		if taskListBucketID != "" {
			filtered := make([]models.PlannerTaskable, 0)
			for _, t := range tasks {
				if deref(t.GetBucketId()) == taskListBucketID {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}

		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(tasks))
			for _, t := range tasks {
				item := map[string]interface{}{
					"id":               deref(t.GetId()),
					"title":            deref(t.GetTitle()),
					"bucket_id":        deref(t.GetBucketId()),
					"percent_complete": 0,
				}
				if t.GetPercentComplete() != nil {
					item["percent_complete"] = *t.GetPercentComplete()
				}
				if t.GetDueDateTime() != nil {
					item["due"] = t.GetDueDateTime().Format(time.RFC3339)
				}
				if t.GetCreatedDateTime() != nil {
					item["created"] = t.GetCreatedDateTime().Format(time.RFC3339)
				}
				if t.GetPriority() != nil {
					item["priority"] = *t.GetPriority()
				}
				items = append(items, item)
			}
			return output.JSON(items)
		case output.FormatPlain:
			var rows [][]string
			for _, t := range tasks {
				pct := "0"
				if t.GetPercentComplete() != nil {
					pct = fmt.Sprintf("%d", *t.GetPercentComplete())
				}
				rows = append(rows, []string{deref(t.GetId()), deref(t.GetTitle()), pct})
			}
			output.Plain(rows)
		default:
			headers := []string{"TITLE", "PROGRESS", "DUE", "ID"}
			var rows [][]string
			for _, t := range tasks {
				progress := "Not started"
				if t.GetPercentComplete() != nil {
					switch *t.GetPercentComplete() {
					case 50:
						progress = "In progress"
					case 100:
						progress = "Complete"
					}
				}
				due := ""
				if t.GetDueDateTime() != nil {
					due = t.GetDueDateTime().Format("2 Jan 2006")
				}
				rows = append(rows, []string{
					deref(t.GetTitle()),
					progress,
					due,
					deref(t.GetId()),
				})
			}
			output.Table(headers, rows)
			output.Success(fmt.Sprintf("%d task(s) found", len(tasks)))
		}
		return nil
	},
}

// ── planner tasks create ──

var (
	taskCreatePlanID   string
	taskCreateBucketID string
	taskCreateTitle    string
	taskCreateAssign   string
	taskCreateDue      string
)

var plannerTasksCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task in a Planner plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskCreatePlanID == "" {
			return fmt.Errorf("--plan is required")
		}
		if taskCreateBucketID == "" {
			return fmt.Errorf("--bucket is required")
		}
		if taskCreateTitle == "" {
			return fmt.Errorf("--title is required")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create task %q in plan %s bucket %s", taskCreateTitle, taskCreatePlanID, taskCreateBucketID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "create_task",
					"title": taskCreateTitle, "plan_id": taskCreatePlanID, "bucket_id": taskCreateBucketID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		task := models.NewPlannerTask()
		task.SetPlanId(ptr(taskCreatePlanID))
		task.SetBucketId(ptr(taskCreateBucketID))
		task.SetTitle(ptr(taskCreateTitle))

		// Handle due date
		if taskCreateDue != "" {
			dueTime, parseErr := time.Parse("2006-01-02", taskCreateDue)
			if parseErr != nil {
				return fmt.Errorf("invalid --due format — use YYYY-MM-DD: %w", parseErr)
			}
			task.SetDueDateTime(&dueTime)
		}

		// Handle assignment
		if taskCreateAssign != "" {
			userID, resolveErr := resolveUserID(ctx, client, taskCreateAssign)
			if resolveErr != nil {
				return resolveErr
			}

			assignments := models.NewPlannerAssignments()
			assignData := map[string]any{
				userID: map[string]any{
					"@odata.type": "microsoft.graph.plannerAssignment",
					"orderHint":   " !",
				},
			}
			assignments.SetAdditionalData(assignData)
			task.SetAssignments(assignments)
		}

		created, err := client.Planner().Tasks().Post(ctx, task, nil)
		if err != nil {
			return fmt.Errorf("creating task: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id":        deref(created.GetId()),
				"title":     deref(created.GetTitle()),
				"plan_id":   deref(created.GetPlanId()),
				"bucket_id": deref(created.GetBucketId()),
			}
			return output.JSON(item)
		default:
			output.Success(fmt.Sprintf("Created task %q (ID: %s)", deref(created.GetTitle()), deref(created.GetId())))
		}
		return nil
	},
}

// ── planner tasks update ──

var (
	taskUpdateID       string
	taskUpdateTitle    string
	taskUpdateProgress string
	taskUpdateAssign   string
	taskUpdateDue      string
)

var plannerTasksUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a Planner task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskUpdateID == "" {
			return fmt.Errorf("--task is required")
		}

		hasUpdate := taskUpdateTitle != "" || taskUpdateProgress != "" || taskUpdateAssign != "" || taskUpdateDue != ""
		if !hasUpdate {
			return fmt.Errorf("at least one of --title, --progress, --assign, or --due is required")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would update task %s", taskUpdateID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "update_task", "id": taskUpdateID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch existing task to get ETag (required by Planner API)
		existing, err := client.Planner().Tasks().ByPlannerTaskId(taskUpdateID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching task for ETag: %w", err)
		}

		etag := getETag(existing.GetAdditionalData())
		if etag == "" {
			return fmt.Errorf("could not extract ETag from task — Planner API requires If-Match for updates")
		}

		// Build update body
		update := models.NewPlannerTask()

		if taskUpdateTitle != "" {
			update.SetTitle(ptr(taskUpdateTitle))
		}

		if taskUpdateProgress != "" {
			pct, pctErr := plannerPercentComplete(taskUpdateProgress)
			if pctErr != nil {
				return pctErr
			}
			update.SetPercentComplete(&pct)
		}

		if taskUpdateDue != "" {
			dueTime, parseErr := time.Parse("2006-01-02", taskUpdateDue)
			if parseErr != nil {
				return fmt.Errorf("invalid --due format — use YYYY-MM-DD: %w", parseErr)
			}
			update.SetDueDateTime(&dueTime)
		}

		if taskUpdateAssign != "" {
			userID, resolveErr := resolveUserID(ctx, client, taskUpdateAssign)
			if resolveErr != nil {
				return resolveErr
			}

			assignments := models.NewPlannerAssignments()
			// Merge with existing assignments
			existingAssignData := make(map[string]any)
			if existing.GetAssignments() != nil {
				for k, v := range existing.GetAssignments().GetAdditionalData() {
					existingAssignData[k] = v
				}
			}
			existingAssignData[userID] = map[string]any{
				"@odata.type": "microsoft.graph.plannerAssignment",
				"orderHint":   " !",
			}
			assignments.SetAdditionalData(existingAssignData)
			update.SetAssignments(assignments)
		}

		// PATCH with If-Match ETag header
		headers := abstractions.NewRequestHeaders()
		headers.Add("If-Match", etag)
		config := &plannerPkg.TasksPlannerTaskItemRequestBuilderPatchRequestConfiguration{
			Headers: headers,
		}

		updated, err := client.Planner().Tasks().ByPlannerTaskId(taskUpdateID).Patch(ctx, update, config)
		if err != nil {
			return fmt.Errorf("updating task: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			item := map[string]interface{}{
				"id": taskUpdateID,
			}
			if updated != nil {
				item["title"] = deref(updated.GetTitle())
				if updated.GetPercentComplete() != nil {
					item["percent_complete"] = *updated.GetPercentComplete()
				}
			}
			return output.JSON(item)
		default:
			output.Success(fmt.Sprintf("Updated task %s", taskUpdateID))
		}
		return nil
	},
}

// ── planner tasks complete ──

var taskCompleteID string

var plannerTasksCompleteCmd = &cobra.Command{
	Use:   "complete",
	Short: "Mark a Planner task as complete (100%)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskCompleteID == "" {
			return fmt.Errorf("--task is required")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would complete task %s", taskCompleteID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "complete_task", "id": taskCompleteID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch for ETag
		existing, err := client.Planner().Tasks().ByPlannerTaskId(taskCompleteID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching task for ETag: %w", err)
		}

		etag := getETag(existing.GetAdditionalData())
		if etag == "" {
			return fmt.Errorf("could not extract ETag from task")
		}

		// Already complete?
		if existing.GetPercentComplete() != nil && *existing.GetPercentComplete() == 100 {
			output.Info("Task is already complete")
			return nil
		}

		update := models.NewPlannerTask()
		pct := int32(100)
		update.SetPercentComplete(&pct)

		headers := abstractions.NewRequestHeaders()
		headers.Add("If-Match", etag)
		config := &plannerPkg.TasksPlannerTaskItemRequestBuilderPatchRequestConfiguration{
			Headers: headers,
		}

		_, err = client.Planner().Tasks().ByPlannerTaskId(taskCompleteID).Patch(ctx, update, config)
		if err != nil {
			return fmt.Errorf("completing task: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id": taskCompleteID, "percent_complete": 100,
			})
		default:
			output.Success(fmt.Sprintf("Completed task %s", taskCompleteID))
		}
		return nil
	},
}

// ── planner tasks delete ──

var (
	taskDeleteID    string
	taskDeleteForce bool
)

var plannerTasksDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a Planner task",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskDeleteID == "" {
			return fmt.Errorf("--task is required")
		}
		if !taskDeleteForce {
			return fmt.Errorf("--force is required to confirm deletion")
		}

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would delete task %s", taskDeleteID))
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "delete_task", "id": taskDeleteID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch for ETag
		existing, err := client.Planner().Tasks().ByPlannerTaskId(taskDeleteID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching task for ETag: %w", err)
		}

		etag := getETag(existing.GetAdditionalData())
		if etag == "" {
			return fmt.Errorf("could not extract ETag from task")
		}

		headers := abstractions.NewRequestHeaders()
		headers.Add("If-Match", etag)
		config := &plannerPkg.TasksPlannerTaskItemRequestBuilderDeleteRequestConfiguration{
			Headers: headers,
		}

		err = client.Planner().Tasks().ByPlannerTaskId(taskDeleteID).Delete(ctx, config)
		if err != nil {
			return fmt.Errorf("deleting task: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"id": taskDeleteID, "deleted": true,
			})
		default:
			output.Success(fmt.Sprintf("Deleted task %s", taskDeleteID))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  Command tree registration
// ──────────────────────────────────────────────

func init() {
	// Plans
	plannerPlansCreateCmd.Flags().StringVar(&planCreateName, "name", "", "Plan name")
	plannerPlansCreateCmd.Flags().StringVar(&planCreateGroupID, "group-id", "", "M365 Group ID that owns the plan")

	plannerPlansCmd.AddCommand(plannerPlansListCmd)
	plannerPlansCmd.AddCommand(plannerPlansCreateCmd)

	// Buckets
	plannerBucketsListCmd.Flags().StringVar(&bucketListPlanID, "plan", "", "Plan ID")
	plannerBucketsCreateCmd.Flags().StringVar(&bucketCreatePlanID, "plan", "", "Plan ID")
	plannerBucketsCreateCmd.Flags().StringVar(&bucketCreateName, "name", "", "Bucket name")

	plannerBucketsCmd.AddCommand(plannerBucketsListCmd)
	plannerBucketsCmd.AddCommand(plannerBucketsCreateCmd)

	// Tasks
	plannerTasksListCmd.Flags().StringVar(&taskListPlanID, "plan", "", "Plan ID")
	plannerTasksListCmd.Flags().StringVar(&taskListBucketID, "bucket", "", "Filter by bucket ID (optional)")

	plannerTasksCreateCmd.Flags().StringVar(&taskCreatePlanID, "plan", "", "Plan ID")
	plannerTasksCreateCmd.Flags().StringVar(&taskCreateBucketID, "bucket", "", "Bucket ID")
	plannerTasksCreateCmd.Flags().StringVar(&taskCreateTitle, "title", "", "Task title")
	plannerTasksCreateCmd.Flags().StringVar(&taskCreateAssign, "assign", "", "Assign to user (email or user ID)")
	plannerTasksCreateCmd.Flags().StringVar(&taskCreateDue, "due", "", "Due date (YYYY-MM-DD)")

	plannerTasksUpdateCmd.Flags().StringVar(&taskUpdateID, "task", "", "Task ID")
	plannerTasksUpdateCmd.Flags().StringVar(&taskUpdateTitle, "title", "", "New title")
	plannerTasksUpdateCmd.Flags().StringVar(&taskUpdateProgress, "progress", "", "Progress: not-started, in-progress, complete")
	plannerTasksUpdateCmd.Flags().StringVar(&taskUpdateAssign, "assign", "", "Add assignee (email or user ID)")
	plannerTasksUpdateCmd.Flags().StringVar(&taskUpdateDue, "due", "", "Due date (YYYY-MM-DD)")

	plannerTasksCompleteCmd.Flags().StringVar(&taskCompleteID, "task", "", "Task ID")

	plannerTasksDeleteCmd.Flags().StringVar(&taskDeleteID, "task", "", "Task ID")
	plannerTasksDeleteCmd.Flags().BoolVar(&taskDeleteForce, "force", false, "Confirm deletion")

	plannerTasksCmd.AddCommand(plannerTasksListCmd)
	plannerTasksCmd.AddCommand(plannerTasksCreateCmd)
	plannerTasksCmd.AddCommand(plannerTasksUpdateCmd)
	plannerTasksCmd.AddCommand(plannerTasksCompleteCmd)
	plannerTasksCmd.AddCommand(plannerTasksDeleteCmd)

	// Wire into parent
	plannerCmd.AddCommand(plannerPlansCmd)
	plannerCmd.AddCommand(plannerBucketsCmd)
	plannerCmd.AddCommand(plannerTasksCmd)
}


