//go:build integration
// +build integration

// Integration tests for Microsoft To Do commands.
// Requires a valid cb365 auth token. Run with:
//   CB365_KEYRING_PASSWORD=... CB365_IPV4_ONLY=1 go test -tags=integration -v ./cmd/cb365/

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// cb365 runs the CLI and returns stdout, stderr, and any error.
func cb365(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(os.Getenv("HOME")+"/.local/bin/cb365", args...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// --- Lists ---

func TestTodoListsList(t *testing.T) {
	out, _, err := cb365(t, "todo", "lists", "list", "--json")
	if err != nil {
		t.Fatalf("todo lists list failed: %v", err)
	}
	var lists []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &lists); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(lists) == 0 {
		t.Fatal("expected at least one list")
	}
	// Every list must have id and display_name
	for _, l := range lists {
		if l["id"] == nil || l["id"] == "" {
			t.Error("list missing id")
		}
		if l["display_name"] == nil || l["display_name"] == "" {
			t.Error("list missing display_name")
		}
	}
}

func TestTodoListsCreateUpdateDelete(t *testing.T) {
	// Create
	out, _, err := cb365(t, "todo", "lists", "create", "--name", "cb365-integration-test", "--json")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	var created map[string]interface{}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	listID, ok := created["id"].(string)
	if !ok || listID == "" {
		t.Fatal("created list missing id")
	}
	if created["display_name"] != "cb365-integration-test" {
		t.Errorf("expected display_name 'cb365-integration-test', got %v", created["display_name"])
	}

	// Update (rename)
	out, _, err = cb365(t, "todo", "lists", "update", "--list", listID, "--name", "cb365-integration-renamed", "--json")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	var updated map[string]interface{}
	if err := json.Unmarshal([]byte(out), &updated); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if updated["display_name"] != "cb365-integration-renamed" {
		t.Errorf("expected renamed to 'cb365-integration-renamed', got %v", updated["display_name"])
	}

	// Delete
	out, _, err = cb365(t, "todo", "lists", "delete", "--list", listID, "--force", "--json")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	var deleted map[string]interface{}
	if err := json.Unmarshal([]byte(out), &deleted); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if deleted["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", deleted["status"])
	}
}

// --- Tasks ---

func TestTodoTasksCRUD(t *testing.T) {
	// Create a task in the default Tasks list
	out, _, err := cb365(t, "todo", "tasks", "create",
		"--list", "Tasks",
		"--title", "cb365 integration test task",
		"--body", "Automated test — safe to delete",
		"--due", "2026-12-31",
		"--json")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	var created map[string]interface{}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	taskID, ok := created["id"].(string)
	if !ok || taskID == "" {
		t.Fatal("created task missing id")
	}
	if created["title"] != "cb365 integration test task" {
		t.Errorf("title mismatch: %v", created["title"])
	}

	// Get
	out, _, err = cb365(t, "todo", "tasks", "get", "--list", "Tasks", "--task", taskID, "--json")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	var fetched map[string]interface{}
	if err := json.Unmarshal([]byte(out), &fetched); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if fetched["title"] != "cb365 integration test task" {
		t.Errorf("get title mismatch: %v", fetched["title"])
	}

	// Update
	out, _, err = cb365(t, "todo", "tasks", "update",
		"--list", "Tasks", "--task", taskID,
		"--title", "cb365 updated test task",
		"--status", "inProgress",
		"--json")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	var updatedTask map[string]interface{}
	if err := json.Unmarshal([]byte(out), &updatedTask); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if updatedTask["title"] != "cb365 updated test task" {
		t.Errorf("update title mismatch: %v", updatedTask["title"])
	}
	if updatedTask["status"] != "inProgress" {
		t.Errorf("update status mismatch: %v", updatedTask["status"])
	}

	// Complete
	out, _, err = cb365(t, "todo", "tasks", "complete", "--list", "Tasks", "--task", taskID, "--json")
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	var completed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &completed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if completed["status"] != "completed" {
		t.Errorf("complete status mismatch: %v", completed["status"])
	}

	// Delete
	out, _, err = cb365(t, "todo", "tasks", "delete", "--list", "Tasks", "--task", taskID, "--force", "--json")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	var deletedTask map[string]interface{}
	if err := json.Unmarshal([]byte(out), &deletedTask); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if deletedTask["status"] != "ok" {
		t.Errorf("delete status mismatch: %v", deletedTask["status"])
	}
}

// --- Name resolution ---

func TestNameToIDResolution(t *testing.T) {
	// "Tasks" is a wellknown list that always exists
	out, _, err := cb365(t, "todo", "tasks", "list", "--list", "Tasks", "--json")
	if err != nil {
		t.Fatalf("name resolution failed: %v", err)
	}
	// Should return valid JSON array
	var tasks []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

// --- Dry-run ---

func TestDryRunDoesNotCreate(t *testing.T) {
	// Count tasks before
	out, _, err := cb365(t, "todo", "tasks", "list", "--list", "Tasks", "--json")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var before []map[string]interface{}
	json.Unmarshal([]byte(out), &before) // #nosec

	// Dry-run create
	out, stderr, err := cb365(t, "todo", "tasks", "create",
		"--list", "Tasks", "--title", "ghost-task-should-not-exist",
		"--dry-run", "--json")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var dryResult map[string]interface{}
	json.Unmarshal([]byte(out), &dryResult) // #nosec
	if dryResult["dry_run"] != true {
		t.Error("expected dry_run=true in output")
	}
	_ = stderr

	// Count tasks after
	out, _, err = cb365(t, "todo", "tasks", "list", "--list", "Tasks", "--json")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var after []map[string]interface{}
	json.Unmarshal([]byte(out), &after) // #nosec

	if len(after) != len(before) {
		t.Errorf("dry-run created a task: before=%d after=%d", len(before), len(after))
	}
}

// --- Force guard ---

func TestForceGuardBlocksDelete(t *testing.T) {
	_, _, err := cb365(t, "todo", "lists", "delete", "--list", "Tasks")
	if err == nil {
		t.Fatal("expected error without --force")
	}
	_, _, err = cb365(t, "todo", "tasks", "delete", "--list", "Tasks", "--task", "fake-id")
	if err == nil {
		t.Fatal("expected error without --force")
	}
}

// --- Security: no tokens in output ---

func TestNoTokensInOutput(t *testing.T) {
	commands := [][]string{
		{"todo", "lists", "list", "--json"},
		{"todo", "lists", "list", "--verbose"},
		{"auth", "status", "--json"},
		{"auth", "profiles", "--json"},
	}
	for _, args := range commands {
		out, stderr, _ := cb365(t, args...)
		combined := out + stderr
		// Access tokens start with "eyJ" (base64 JWT header)
		if strings.Contains(combined, "eyJ") && len(combined) > 100 {
			// Check for JWT pattern (three dot-separated base64 segments)
			for _, word := range strings.Fields(combined) {
				parts := strings.Split(word, ".")
				if len(parts) == 3 && strings.HasPrefix(parts[0], "eyJ") {
					t.Errorf("possible JWT token in output of %v", args)
				}
			}
		}
	}
}
