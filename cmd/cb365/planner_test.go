package main

import (
	"testing"
)

// ──────────────────────────────────────────────
//  Planner safety/helper unit tests
// ──────────────────────────────────────────────

func TestPlannerPercentComplete(t *testing.T) {
	tests := []struct {
		input   string
		want    int32
		wantErr bool
	}{
		{"not-started", 0, false},
		{"notstarted", 0, false},
		{"0", 0, false},
		{"in-progress", 50, false},
		{"inprogress", 50, false},
		{"50", 50, false},
		{"complete", 100, false},
		{"completed", 100, false},
		{"done", 100, false},
		{"100", 100, false},
		{"invalid", 0, true},
		{"", 0, true},
		{"75", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := plannerPercentComplete(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("plannerPercentComplete(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("plannerPercentComplete(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetETag(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		want string
	}{
		{
			name: "string value",
			data: map[string]any{"@odata.etag": "W/\"abc123\""},
			want: "W/\"abc123\"",
		},
		{
			name: "pointer value",
			data: func() map[string]any {
				s := "W/\"def456\""
				return map[string]any{"@odata.etag": &s}
			}(),
			want: "W/\"def456\"",
		},
		{
			name: "missing key",
			data: map[string]any{"other": "value"},
			want: "",
		},
		{
			name: "nil map",
			data: nil,
			want: "",
		},
		{
			name: "nil string pointer",
			data: map[string]any{"@odata.etag": (*string)(nil)},
			want: "",
		},
		{
			name: "non-string value",
			data: map[string]any{"@odata.etag": 42},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getETag(tt.data)
			if got != tt.want {
				t.Errorf("getETag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlannerDeleteRequiresForce(t *testing.T) {
	// Verify the delete command rejects without --force
	// This tests the validation logic, not the Graph API call
	cmd := plannerTasksDeleteCmd
	cmd.SetArgs([]string{"--task", "fake-id"})

	// Reset the force flag to false
	taskDeleteForce = false
	taskDeleteID = "fake-id"

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Error("expected error when --force not set, got nil")
	}
	if err != nil && err.Error() != "--force is required to confirm deletion" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestPlannerDryRunSkipsAPI(t *testing.T) {
	// Verify --dry-run returns early without making API calls
	// If it tried to call the API, it would fail (no auth in tests)

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "plans create dry-run",
			fn: func() error {
				planCreateName = "test"
				planCreateGroupID = "fake-group"
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerPlansCreateCmd.RunE(plannerPlansCreateCmd, []string{})
			},
		},
		{
			name: "buckets create dry-run",
			fn: func() error {
				bucketCreatePlanID = "fake-plan"
				bucketCreateName = "test"
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerBucketsCreateCmd.RunE(plannerBucketsCreateCmd, []string{})
			},
		},
		{
			name: "tasks create dry-run",
			fn: func() error {
				taskCreatePlanID = "fake-plan"
				taskCreateBucketID = "fake-bucket"
				taskCreateTitle = "test"
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerTasksCreateCmd.RunE(plannerTasksCreateCmd, []string{})
			},
		},
		{
			name: "tasks update dry-run",
			fn: func() error {
				taskUpdateID = "fake-task"
				taskUpdateTitle = "test"
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerTasksUpdateCmd.RunE(plannerTasksUpdateCmd, []string{})
			},
		},
		{
			name: "tasks complete dry-run",
			fn: func() error {
				taskCompleteID = "fake-task"
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerTasksCompleteCmd.RunE(plannerTasksCompleteCmd, []string{})
			},
		},
		{
			name: "tasks delete dry-run",
			fn: func() error {
				taskDeleteID = "fake-task"
				taskDeleteForce = true
				flagDryRun = true
				defer func() { flagDryRun = false }()
				return plannerTasksDeleteCmd.RunE(plannerTasksDeleteCmd, []string{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != nil {
				t.Errorf("dry-run should succeed without API, got error: %v", err)
			}
		})
	}
}

func TestPlannerValidation(t *testing.T) {
	// Test that required flags are validated before any API call

	tests := []struct {
		name    string
		fn      func() error
		wantMsg string
	}{
		{
			name: "plans create missing name",
			fn: func() error {
				planCreateName = ""
				planCreateGroupID = "some-group"
				return plannerPlansCreateCmd.RunE(plannerPlansCreateCmd, []string{})
			},
			wantMsg: "--name is required",
		},
		{
			name: "plans create missing group-id",
			fn: func() error {
				planCreateName = "test"
				planCreateGroupID = ""
				return plannerPlansCreateCmd.RunE(plannerPlansCreateCmd, []string{})
			},
			wantMsg: "--group-id is required — plans must be owned by an M365 Group",
		},
		{
			name: "buckets list missing plan",
			fn: func() error {
				bucketListPlanID = ""
				return plannerBucketsListCmd.RunE(plannerBucketsListCmd, []string{})
			},
			wantMsg: "--plan is required",
		},
		{
			name: "tasks create missing title",
			fn: func() error {
				taskCreatePlanID = "p"
				taskCreateBucketID = "b"
				taskCreateTitle = ""
				return plannerTasksCreateCmd.RunE(plannerTasksCreateCmd, []string{})
			},
			wantMsg: "--title is required",
		},
		{
			name: "tasks update no fields",
			fn: func() error {
				taskUpdateID = "t"
				taskUpdateTitle = ""
				taskUpdateProgress = ""
				taskUpdateAssign = ""
				taskUpdateDue = ""
				return plannerTasksUpdateCmd.RunE(plannerTasksUpdateCmd, []string{})
			},
			wantMsg: "at least one of --title, --progress, --assign, or --due is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Errorf("expected error %q, got nil", tt.wantMsg)
				return
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

