//go:build integration

// Package integration contains end-to-end tests that make real Microsoft Graph API calls.
//
// These tests require:
//   - A valid cb365 profile with delegated auth
//   - CB365_TEST_PROFILE environment variable set to the profile name
//   - CB365_TEST_LIST environment variable set to a To Do list name for task tests
//
// Run with:
//
//	go test -tags integration -v ./test/integration/
//
// These tests are NOT run in CI — they require live credentials and a real M365 tenant.
package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var (
	testProfile string
	testList    string
)

func TestMain(m *testing.M) {
	testProfile = os.Getenv("CB365_TEST_PROFILE")
	if testProfile == "" {
		testProfile = "work-delegated"
	}
	testList = os.Getenv("CB365_TEST_LIST")
	if testList == "" {
		testList = "Tasks"
	}
	os.Exit(m.Run())
}

func cb365(t *testing.T, args ...string) (string, error) {
	t.Helper()
	fullArgs := append(args, "--profile", testProfile)
	cmd := exec.Command("cb365", fullArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func cb365JSON(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return cb365(t, append(args, "--json")...)
}

// --- Auth ---

func TestAuthStatus(t *testing.T) {
	out, err := cb365JSON(t, "auth", "status")
	if err != nil {
		t.Fatalf("auth status failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "expires") && !strings.Contains(out, "valid") {
		t.Logf("auth status output: %s", out)
	}
}

func TestAuthProfiles(t *testing.T) {
	out, err := cb365JSON(t, "auth", "profiles")
	if err != nil {
		t.Fatalf("auth profiles failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, testProfile) {
		t.Errorf("expected profile %q in output, got: %s", testProfile, out)
	}
}

// --- To Do ---

func TestTodoListsList(t *testing.T) {
	out, err := cb365JSON(t, "todo", "lists", "list")
	if err != nil {
		t.Fatalf("todo lists list failed: %s\n%s", err, out)
	}
	var lists []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &lists); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, out)
	}
	if len(lists) == 0 {
		t.Error("expected at least one task list")
	}
}

func TestTodoTasksLifecycle(t *testing.T) {
	// Create
	out, err := cb365JSON(t, "todo", "tasks", "create",
		"--list", testList,
		"--title", "cb365 integration test task",
		"--body", "This task was created by the integration test suite. Safe to delete.")
	if err != nil {
		t.Fatalf("todo tasks create failed: %s\n%s", err, out)
	}

	var created map[string]interface{}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("failed to parse created task JSON: %s", err)
	}
	taskID, ok := created["id"].(string)
	if !ok || taskID == "" {
		t.Fatalf("created task has no id: %v", created)
	}
	t.Logf("created task: %s", taskID)

	// Complete
	out, err = cb365(t, "todo", "tasks", "complete", "--list", testList, "--task", taskID)
	if err != nil {
		t.Errorf("todo tasks complete failed: %s\n%s", err, out)
	}

	// Delete
	out, err = cb365(t, "todo", "tasks", "delete", "--list", testList, "--task", taskID, "--force")
	if err != nil {
		t.Errorf("todo tasks delete failed: %s\n%s", err, out)
	}
}

// --- Mail ---

func TestMailList(t *testing.T) {
	out, err := cb365JSON(t, "mail", "list")
	if err != nil {
		t.Fatalf("mail list failed: %s\n%s", err, out)
	}
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &messages); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, out)
	}
	// Mail list may be empty — that's OK
	t.Logf("found %d messages", len(messages))
}

func TestMailSearch(t *testing.T) {
	out, err := cb365JSON(t, "mail", "search", "--query", "test")
	if err != nil {
		t.Fatalf("mail search failed: %s\n%s", err, out)
	}
	t.Logf("search returned: %d bytes", len(out))
}

// --- Calendar ---

func TestCalendarList(t *testing.T) {
	out, err := cb365JSON(t, "calendar", "list", "--from", "2026-04-01", "--to", "2026-04-07")
	if err != nil {
		t.Fatalf("calendar list failed: %s\n%s", err, out)
	}
	var events []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &events); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, out)
	}
	t.Logf("found %d events", len(events))
}

// --- Contacts ---

func TestContactsList(t *testing.T) {
	out, err := cb365JSON(t, "contacts", "list")
	if err != nil {
		t.Fatalf("contacts list failed: %s\n%s", err, out)
	}
	t.Logf("contacts list returned: %d bytes", len(out))
}

func TestContactsSearch(t *testing.T) {
	out, err := cb365JSON(t, "contacts", "search", "--query", "test")
	if err != nil {
		t.Fatalf("contacts search failed: %s\n%s", err, out)
	}
	t.Logf("contacts search returned: %d bytes", len(out))
}

// --- Planner ---

func TestPlannerPlansList(t *testing.T) {
	out, err := cb365JSON(t, "planner", "plans", "list")
	if err != nil {
		t.Fatalf("planner plans list failed: %s\n%s", err, out)
	}
	t.Logf("planner plans list returned: %d bytes", len(out))
}

// --- Teams ---

func TestTeamsChannelsList(t *testing.T) {
	teamName := os.Getenv("CB365_TEST_TEAM")
	if teamName == "" {
		t.Skip("CB365_TEST_TEAM not set — skipping Teams test")
	}
	out, err := cb365JSON(t, "teams", "channels", "list", "--team", teamName)
	if err != nil {
		t.Fatalf("teams channels list failed: %s\n%s", err, out)
	}
	t.Logf("found channels: %d bytes", len(out))
}

func TestTeamsChatList(t *testing.T) {
	out, err := cb365JSON(t, "teams", "chat", "list")
	if err != nil {
		t.Fatalf("teams chat list failed: %s\n%s", err, out)
	}
	t.Logf("teams chat list returned: %d bytes", len(out))
}

// --- SharePoint ---

func TestSharePointSitesList(t *testing.T) {
	out, err := cb365JSON(t, "sharepoint", "sites", "list")
	if err != nil {
		t.Fatalf("sharepoint sites list failed: %s\n%s", err, out)
	}
	t.Logf("sharepoint sites list returned: %d bytes", len(out))
}

// --- OneDrive ---

func TestOneDriveLs(t *testing.T) {
	out, err := cb365JSON(t, "onedrive", "ls")
	if err != nil {
		t.Fatalf("onedrive ls failed: %s\n%s", err, out)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, out)
	}
	t.Logf("found %d items in OneDrive root", len(items))
}

// --- Version ---

func TestVersionJSON(t *testing.T) {
	out, err := cb365JSON(t, "version")
	if err != nil {
		t.Fatalf("version --json failed: %s\n%s", err, out)
	}
	var info map[string]string
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("failed to parse version JSON: %s", err)
	}
	if info["url"] != "https://github.com/nz365guy/cb365" {
		t.Errorf("unexpected url: %s", info["url"])
	}
}

// --- Dry-Run Safety ---

func TestDryRunDoesNotCreate(t *testing.T) {
	// Verify --dry-run prevents task creation
	out, err := cb365(t, "todo", "tasks", "create",
		"--list", testList,
		"--title", "SHOULD NOT EXIST — dry-run test",
		"--dry-run")
	if err != nil {
		t.Fatalf("dry-run create failed unexpectedly: %s\n%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "dry") {
		t.Logf("dry-run output: %s", out)
	}

	// Verify the task was NOT created
	listOut, err := cb365JSON(t, "todo", "tasks", "list", "--list", testList)
	if err != nil {
		t.Fatalf("todo tasks list failed: %s\n%s", err, listOut)
	}
	if strings.Contains(listOut, "SHOULD NOT EXIST") {
		t.Error("dry-run task was actually created — safety violation!")
	}
}

