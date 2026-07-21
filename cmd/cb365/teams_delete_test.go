package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nz365guy/cb365/internal/auth"
	"github.com/nz365guy/cb365/internal/config"
)

var validTeamsDeleteTarget = auth.ManagedChannelMessageTarget{
	TeamID:    "11111111-2222-3333-4444-555555555555",
	ChannelID: "19:abcDEF_123=-@thread.tacv2",
	MessageID: "1750000000000",
}

type teamsDeleteRecorder struct {
	t            *testing.T
	loadCalls    int
	verifyCalls  int
	tokenCalls   int
	postCalls    int
	consumeCalls int
	auditCalls   int

	cfg        *config.Config
	profile    *config.Profile
	verifyErr  error
	tokenErr   error
	httpResult teamsDeleteHTTPResult
	consumeErr error
	auditErr   error
	event      teamsDeleteAuditEvent
	verified   auth.ManagedChannelMessageTarget
	posted     auth.ManagedChannelMessageTarget
	consumed   auth.ManagedChannelMessageTarget
}

func newTeamsDeleteRecorder(t *testing.T) *teamsDeleteRecorder {
	profile := &config.Profile{
		Name:     "managed-test",
		TenantID: "tenant-test",
		ClientID: "client-test",
		AuthMode: config.AuthModeDelegated,
		Scopes:   []string{teamsDeleteScope},
		ManagedDelegated: &config.ManagedDelegatedMetadata{
			HomeAccountID:  "account-test",
			SecretID:       "cache-secret",
			OrganisationID: "org-test",
			ProjectID:      "project-test",
			AssignedHost:   "host-test",
			MigrationState: "complete",
			ChannelMessageProvenance: map[string]string{
				auth.ManagedChannelMessageProvenanceKey(validTeamsDeleteTarget): "provenance-secret",
			},
		},
	}
	return &teamsDeleteRecorder{
		t:          t,
		cfg:        &config.Config{Profiles: map[string]*config.Profile{profile.Name: profile}},
		profile:    profile,
		httpResult: teamsDeleteHTTPResult{Attempted: true, Status: http.StatusNoContent, CorrelationID: "correlation-test"},
	}
}

func (r *teamsDeleteRecorder) dependencies() teamsDeleteDependencies {
	return teamsDeleteDependencies{
		loadProfile: func() (*config.Config, string, *config.Profile, error) {
			r.loadCalls++
			return r.cfg, r.profile.Name, r.profile, nil
		},
		verifyProvenance: func(_ context.Context, _ *config.Profile, reference string, target auth.ManagedChannelMessageTarget) error {
			r.verifyCalls++
			r.verified = target
			if reference != "provenance-secret" {
				r.t.Fatalf("unexpected provenance reference %q", reference)
			}
			return r.verifyErr
		},
		acquireToken: func(context.Context, *config.Config, *config.Profile) (string, error) {
			r.tokenCalls++
			return "opaque-token-sentinel", r.tokenErr
		},
		post: func(_ context.Context, target auth.ManagedChannelMessageTarget, token string, _ bool) teamsDeleteHTTPResult {
			r.postCalls++
			r.posted = target
			if token != "opaque-token-sentinel" {
				r.t.Fatal("delete path did not use the acquired in-memory token")
			}
			return r.httpResult
		},
		consume: func(_ context.Context, _ *config.Config, _ *config.Profile, reference string, target auth.ManagedChannelMessageTarget) error {
			r.consumeCalls++
			r.consumed = target
			if reference != "provenance-secret" {
				r.t.Fatalf("unexpected consumed provenance reference %q", reference)
			}
			return r.consumeErr
		},
		audit: func(event teamsDeleteAuditEvent) error {
			r.auditCalls++
			r.event = event
			return r.auditErr
		},
		now: func() time.Time { return time.Date(2026, 7, 21, 11, 0, 0, 0, time.UTC) },
	}
}

func TestTeamsDeleteLocalGuardsMakeZeroDependencyCalls(t *testing.T) {
	tests := []struct {
		name    string
		options teamsDeleteOptions
		wantErr bool
	}{
		{name: "invalid team", options: teamsDeleteOptions{Target: auth.ManagedChannelMessageTarget{TeamID: "*", ChannelID: validTeamsDeleteTarget.ChannelID, MessageID: validTeamsDeleteTarget.MessageID}, Confirm: true}, wantErr: true},
		{name: "invalid channel list", options: teamsDeleteOptions{Target: auth.ManagedChannelMessageTarget{TeamID: validTeamsDeleteTarget.TeamID, ChannelID: validTeamsDeleteTarget.ChannelID + ",other", MessageID: validTeamsDeleteTarget.MessageID}, Confirm: true}, wantErr: true},
		{name: "invalid message wildcard", options: teamsDeleteOptions{Target: auth.ManagedChannelMessageTarget{TeamID: validTeamsDeleteTarget.TeamID, ChannelID: validTeamsDeleteTarget.ChannelID, MessageID: "*"}, Confirm: true}, wantErr: true},
		{name: "unconfirmed", options: teamsDeleteOptions{Target: validTeamsDeleteTarget}, wantErr: true},
		{name: "dry run", options: teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true, DryRun: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := newTeamsDeleteRecorder(t)
			result, err := executeTeamsDelete(context.Background(), test.options, recorder.dependencies())
			if test.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.options.DryRun && result.Class != "dry_run" {
				t.Fatalf("dry-run class = %q", result.Class)
			}
			if recorder.loadCalls+recorder.verifyCalls+recorder.tokenCalls+recorder.postCalls+recorder.consumeCalls+recorder.auditCalls != 0 {
				t.Fatalf("local guard crossed a dependency boundary: %+v", recorder)
			}
		})
	}
}

func TestTeamsDeleteRejectsUnsupportedProfilesBeforeCredentialOrGraph(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Profile)
	}{
		{name: "app only", mutate: func(profile *config.Profile) { profile.AuthMode = config.AuthModeAppOnly }},
		{name: "legacy delegated", mutate: func(profile *config.Profile) { profile.ManagedDelegated = nil }},
		{name: "missing scope", mutate: func(profile *config.Profile) { profile.Scopes = []string{"Calendars.Read"} }},
		{name: "broader read scope", mutate: func(profile *config.Profile) { profile.Scopes = []string{teamsDeleteScope, "ChannelMessage.Read.All"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := newTeamsDeleteRecorder(t)
			test.mutate(recorder.profile)
			_, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
			if err == nil {
				t.Fatal("expected profile rejection")
			}
			if recorder.verifyCalls != 0 || recorder.tokenCalls != 0 || recorder.postCalls != 0 || recorder.consumeCalls != 0 {
				t.Fatalf("profile rejection crossed a credential or Graph boundary: %+v", recorder)
			}
			if recorder.auditCalls != 1 || recorder.event.ResultClass != "profile_rejected" {
				t.Fatalf("profile rejection audit = %+v", recorder.event)
			}
		})
	}
}

func TestTeamsDeleteFailsClosedBeforeGraphOnProvenanceOrTokenFailure(t *testing.T) {
	t.Run("missing provenance", func(t *testing.T) {
		recorder := newTeamsDeleteRecorder(t)
		recorder.profile.ManagedDelegated.ChannelMessageProvenance = nil
		_, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
		if err == nil || recorder.verifyCalls != 0 || recorder.tokenCalls != 0 || recorder.postCalls != 0 {
			t.Fatalf("missing provenance did not fail locally: err=%v recorder=%+v", err, recorder)
		}
	})
	t.Run("rejected provenance", func(t *testing.T) {
		recorder := newTeamsDeleteRecorder(t)
		recorder.verifyErr = errors.New("raw BWS response sentinel")
		_, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
		if err == nil || strings.Contains(err.Error(), "raw BWS") || recorder.tokenCalls != 0 || recorder.postCalls != 0 {
			t.Fatalf("provenance rejection was unsafe: err=%v recorder=%+v", err, recorder)
		}
	})
	t.Run("token failure", func(t *testing.T) {
		recorder := newTeamsDeleteRecorder(t)
		recorder.tokenErr = errors.New("raw identity response sentinel")
		_, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
		if err == nil || strings.Contains(err.Error(), "raw identity") || recorder.postCalls != 0 || recorder.consumeCalls != 0 {
			t.Fatalf("token failure was unsafe: err=%v recorder=%+v", err, recorder)
		}
	})
}

func TestTeamsDeleteSuccessUsesExactTargetOnceAndConsumesProvenance(t *testing.T) {
	recorder := newTeamsDeleteRecorder(t)
	result, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Class != "success" || result.Status != http.StatusNoContent || result.CorrelationID != "correlation-test" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if recorder.verifyCalls != 1 || recorder.tokenCalls != 1 || recorder.postCalls != 1 || recorder.consumeCalls != 1 || recorder.auditCalls != 1 {
		t.Fatalf("unexpected call counts: %+v", recorder)
	}
	if recorder.verified != validTeamsDeleteTarget || recorder.posted != validTeamsDeleteTarget || recorder.consumed != validTeamsDeleteTarget {
		t.Fatalf("target binding changed: verify=%+v post=%+v consume=%+v", recorder.verified, recorder.posted, recorder.consumed)
	}
	encoded, _ := json.Marshal(recorder.event)
	if strings.Contains(string(encoded), "opaque-token-sentinel") || recorder.event.ResultClass != "success" {
		t.Fatalf("unsafe audit event: %s", encoded)
	}
}

func TestTeamsDeleteAmbiguousAndHTTPFailuresNeverRetry(t *testing.T) {
	tests := []struct {
		name       string
		httpResult teamsDeleteHTTPResult
		wantClass  string
	}{
		{name: "transport ambiguity", httpResult: teamsDeleteHTTPResult{Attempted: true, Err: errors.New("raw transport response sentinel")}, wantClass: "ambiguous"},
		{name: "unauthorized", httpResult: teamsDeleteHTTPResult{Attempted: true, Status: http.StatusUnauthorized}, wantClass: "unauthorized"},
		{name: "forbidden", httpResult: teamsDeleteHTTPResult{Attempted: true, Status: http.StatusForbidden}, wantClass: "forbidden"},
		{name: "not found", httpResult: teamsDeleteHTTPResult{Attempted: true, Status: http.StatusNotFound}, wantClass: "not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := newTeamsDeleteRecorder(t)
			recorder.httpResult = test.httpResult
			result, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
			if err == nil || result.Class != test.wantClass {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if strings.Contains(err.Error(), "raw") || recorder.postCalls != 1 || recorder.consumeCalls != 1 || recorder.auditCalls != 1 {
				t.Fatalf("unsafe failure handling: err=%v recorder=%+v", err, recorder)
			}
		})
	}
}

func TestTeamsDeleteAuditOrRetirementFailureDoesNotRepeatPost(t *testing.T) {
	t.Run("retirement", func(t *testing.T) {
		recorder := newTeamsDeleteRecorder(t)
		recorder.consumeErr = errors.New("provider response sentinel")
		result, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
		if err == nil || result.Class != "provenance_retirement_failed" || recorder.postCalls != 1 || recorder.consumeCalls != 1 {
			t.Fatalf("retirement failure was unsafe: result=%+v err=%v recorder=%+v", result, err, recorder)
		}
	})
	t.Run("audit", func(t *testing.T) {
		recorder := newTeamsDeleteRecorder(t)
		recorder.auditErr = errors.New("disk response sentinel")
		_, err := executeTeamsDelete(context.Background(), teamsDeleteOptions{Target: validTeamsDeleteTarget, Confirm: true}, recorder.dependencies())
		if err == nil || strings.Contains(err.Error(), "disk response") || recorder.postCalls != 1 || recorder.auditCalls != 1 {
			t.Fatalf("audit failure was unsafe: err=%v recorder=%+v", err, recorder)
		}
	})
}

func TestPostTeamsDeleteOnceWithClientUsesOneExactPOSTAndIgnoresBody(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost {
			t.Errorf("method = %s", request.Method)
		}
		wantPath := "/teams/" + validTeamsDeleteTarget.TeamID + "/channels/" + validTeamsDeleteTarget.ChannelID + "/messages/" + validTeamsDeleteTarget.MessageID + "/softDelete"
		if request.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", request.URL.Path, wantPath)
		}
		if request.Header.Get("Authorization") != "Bearer opaque-token-sentinel" {
			t.Error("missing in-memory bearer header")
		}
		response.Header().Set("request-id", "request-test")
		response.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(response, "sensitive Graph body sentinel")
	}))
	defer server.Close()
	result := postTeamsDeleteOnceWithClient(context.Background(), validTeamsDeleteTarget, "opaque-token-sentinel", server.URL, server.Client())
	if requests != 1 || !result.Attempted || result.Status != http.StatusForbidden || result.CorrelationID != "request-test" || result.Err != nil {
		t.Fatalf("unexpected one-shot result: requests=%d result=%+v", requests, result)
	}
}

func TestTeamsDeleteCommandContract(t *testing.T) {
	if teamsChannelsDeleteMessageCmd.Flags().Lookup("team") == nil ||
		teamsChannelsDeleteMessageCmd.Flags().Lookup("channel") == nil ||
		teamsChannelsDeleteMessageCmd.Flags().Lookup("message") == nil ||
		teamsChannelsDeleteMessageCmd.Flags().Lookup("confirm") == nil {
		t.Fatal("delete-message is missing a required target or confirmation flag")
	}
	if teamsChannelsDeleteMessageCmd.Flags().Lookup("confirm").DefValue != "false" {
		t.Fatal("delete-message --confirm must default false")
	}
}

func TestAppendTeamsDeleteAuditUsesPrivateMetadataOnlyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	event := teamsDeleteAuditEvent{
		Timestamp:   "2026-07-21T11:00:00Z",
		Operation:   teamsDeleteOperation,
		TenantID:    "tenant",
		Profile:     "sha256:profile",
		TeamID:      validTeamsDeleteTarget.TeamID,
		ChannelID:   validTeamsDeleteTarget.ChannelID,
		MessageID:   validTeamsDeleteTarget.MessageID,
		ResultClass: "success",
		HTTPStatus:  http.StatusNoContent,
	}
	if err := appendTeamsDeleteAudit(event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", config.AppName, "teams-delete-audit.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("audit mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed test path under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "opaque-token-sentinel") || strings.Contains(string(data), "message body") {
		t.Fatalf("audit contains protected content: %s", data)
	}
	var decoded teamsDeleteAuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil || decoded != event {
		t.Fatalf("audit round trip: decoded=%+v err=%v", decoded, err)
	}
}

func TestAppendTeamsDeleteAuditRejectsUnsafeFileMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, ".config", config.AppName)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "teams-delete-audit.jsonl")
	if err := os.WriteFile(path, []byte("unsafe\n"), 0644); err != nil { // #nosec G306 -- intentionally unsafe negative-test fixture
		t.Fatal(err)
	}
	if err := appendTeamsDeleteAudit(teamsDeleteAuditEvent{ResultClass: "test"}); err == nil {
		t.Fatal("mode-0644 audit file was accepted")
	}
}
