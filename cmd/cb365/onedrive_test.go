package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
//  OneDrive safety and structure tests
// ──────────────────────────────────────────────

func TestOnedriveCommandStructure(t *testing.T) {
	if !onedriveCmd.HasSubCommands() {
		t.Fatal("onedrive command should have subcommands")
	}

	found := map[string]bool{}
	for _, sub := range onedriveCmd.Commands() {
		found[sub.Name()] = true
	}

	for _, expected := range []string{"ls", "get", "upload"} {
		if !found[expected] {
			t.Errorf("onedrive missing subcommand %q", expected)
		}
	}
}

func TestOnedriveAliases(t *testing.T) {
	if len(onedriveCmd.Aliases) == 0 {
		t.Fatal("onedrive should have alias 'od'")
	}
	found := false
	for _, a := range onedriveCmd.Aliases {
		if a == "od" {
			found = true
		}
	}
	if !found {
		t.Error("onedrive missing alias 'od'")
	}
}

func TestOnedriveGetRequiresOutput(t *testing.T) {
	cmd := onedriveGetCmd
	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("onedrive get missing --output flag")
	}
}

func TestOnedriveGetForceDefault(t *testing.T) {
	cmd := onedriveGetCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive get missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestOnedriveUploadRequiresFileAndPath(t *testing.T) {
	cmd := onedriveUploadCmd
	fileFlag := cmd.Flags().Lookup("file")
	if fileFlag == nil {
		t.Fatal("onedrive upload missing --file flag")
	}
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive upload missing --path flag")
	}
}

func TestOnedriveUploadForceDefault(t *testing.T) {
	cmd := onedriveUploadCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive upload missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestHumanFileSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{4194304, "4.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		result := humanFileSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("humanFileSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestOneDriveUploadSizeLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int64
		want bool
	}{
		{"empty", 0, false},
		{"simple boundary", odSimpleUploadLimit, true},
		{"session boundary", odMaxUploadSize, true},
		{"above session boundary", odMaxUploadSize + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOneDriveUploadSize(tc.size); (err == nil) != tc.want {
				t.Fatalf("validateOneDriveUploadSize(%d) error=%v, want success=%t", tc.size, err, tc.want)
			}
		})
	}
}

func TestOneDriveUploadSessionChunksWithoutBearerAuth(t *testing.T) {
	const size = odUploadChunkSize + 17
	content := make([]byte, size)
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut {
			t.Fatalf("method=%s, want PUT", r.Method)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("upload session PUT must not include an Authorization header")
		}
		start := int64(calls-1) * odUploadChunkSize
		end := min(int64(calls)*odUploadChunkSize, size) - 1
		wantRange := fmt.Sprintf("bytes %d-%d/%d", start, end, size)
		if got := r.Header.Get("Content-Range"); got != wantRange {
			t.Fatalf("Content-Range=%q, want %q", got, wantRange)
		}
		chunk, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(chunk)) != min(odUploadChunkSize, size-start) {
			t.Fatalf("chunk length=%d", len(chunk))
		}
		if calls == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"synthetic-id","name":"asset.mp4","webUrl":"https://example.test/asset.mp4"}`)
	}))
	defer server.Close()

	item, err := uploadOneDriveSession(context.Background(), server.Client(), server.URL, bytes.NewReader(content), size)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("requests=%d, want 2", calls)
	}
	if item.ID != "synthetic-id" || item.Name != "asset.mp4" {
		t.Fatalf("unexpected completed item: %#v", item)
	}
}

func TestOneDriveUploadSessionRejectsNonHTTPSURL(t *testing.T) {
	if _, err := uploadOneDriveSession(context.Background(), http.DefaultClient, "http://example.test/upload", strings.NewReader("x"), 1); err == nil {
		t.Fatal("expected non-HTTPS upload URL to be rejected")
	}
}

func TestOneDriveUploadRequiresCompleteResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		item odUploadedItem
	}{
		{"missing identity", odUploadedItem{}},
		{"missing name", odUploadedItem{ID: "id", WebURL: "https://example.test/file"}},
		{"non-HTTPS web URL", odUploadedItem{ID: "id", Name: "file", WebURL: "http://example.test/file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOneDriveUploadedItem(&tc.item); err == nil {
				t.Fatal("expected incomplete upload response to be rejected")
			}
		})
	}
	valid := &odUploadedItem{ID: "id", Name: "file", WebURL: "https://example.test/file"}
	if err := validateOneDriveUploadedItem(valid); err != nil {
		t.Fatalf("valid uploaded item rejected: %v", err)
	}
}

func TestOneDriveUploadSessionDoesNotAcceptEmptySuccessBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	if _, err := uploadOneDriveSession(context.Background(), server.Client(), server.URL, strings.NewReader("x"), 1); err == nil {
		t.Fatal("expected empty drive item response to be rejected")
	}
}

type uploadTestRoundTripper func(*http.Request) (*http.Response, error)

func (rt uploadTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

type closeErrorBody struct{ io.Reader }

func (closeErrorBody) Close() error { return fmt.Errorf("signed capability must not escape") }

func TestOneDriveUploadSessionReportsResponseCloseFailureWithoutDetails(t *testing.T) {
	client := &http.Client{Transport: uploadTestRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       closeErrorBody{Reader: strings.NewReader(`{"id":"id","name":"file","webUrl":"https://example.test/file"}`)},
			Header:     make(http.Header),
		}, nil
	})}
	_, err := uploadOneDriveSession(context.Background(), client, "https://example.test/upload", strings.NewReader("x"), 1)
	if err == nil || err.Error() != "closing upload response failed" {
		t.Fatalf("expected bounded response close error, got %v", err)
	}
}

func TestOnedriveLsPathAndItemIDFlags(t *testing.T) {
	cmd := onedriveLsCmd
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive ls missing --path flag")
	}
	itemIDFlag := cmd.Flags().Lookup("item-id")
	if itemIDFlag == nil {
		t.Fatal("onedrive ls missing --item-id flag")
	}
}

func TestOnedriveDeleteRequiresForce(t *testing.T) {
	cmd := onedriveDeleteCmd
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("onedrive delete missing --force flag")
	}
	if forceFlag.DefValue != "false" {
		t.Errorf("--force default should be false, got %s", forceFlag.DefValue)
	}
}

func TestOnedriveMkdirRequiresPath(t *testing.T) {
	cmd := onedriveMkdirCmd
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("onedrive mkdir missing --path flag")
	}
}

func TestOnedriveCommandStructureWithNewCmds(t *testing.T) {
	found := map[string]bool{}
	for _, sub := range onedriveCmd.Commands() {
		found[sub.Name()] = true
	}
	for _, expected := range []string{"ls", "get", "upload", "delete", "mkdir"} {
		if !found[expected] {
			t.Errorf("onedrive missing subcommand %q", expected)
		}
	}
}
