package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	kiota "github.com/microsoft/kiota-abstractions-go"
	khttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
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
		{"45 MiB boundary", odMaxUploadSize, true},
		{"above 45 MiB boundary", odMaxUploadSize + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOneDriveUploadSize(tc.size); (err == nil) != tc.want {
				t.Fatalf("validateOneDriveUploadSize(%d) error=%v, want success=%t", tc.size, err, tc.want)
			}
		})
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

func TestOneDriveContentPutRequestAndReceipt(t *testing.T) {
	content := []byte("synthetic-media-bytes")
	var capturedURL string
	var capturedMethod string
	var capturedContentType string
	var capturedBody []byte
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		capturedURL = request.URL.String()
		capturedMethod = request.Method
		capturedContentType = request.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(request.Body)
		responseBody := `{"id":"synthetic-id","name":"asset #1.mp4","webUrl":"https://example.test/asset"}`
		return &http.Response{
			StatusCode:    http.StatusCreated,
			ContentLength: int64(len(responseBody)),
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(responseBody)),
			Request:       request,
		}, nil
	})
	adapter, err := khttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		noOpGraphAuth{}, nil, nil, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	client := msgraphsdk.NewGraphServiceClient(adapter)
	client.BaseRequestBuilder.PathParameters["baseurl"] = "https://graph.microsoft.com/v1.0"
	itemByPath := "root:/90 Day Mentoring Challenge/Testimonial Library/asset #1.mp4:"
	uploaded, err := client.Drives().ByDriveId("drive-id").Items().ByDriveItemId(itemByPath).Content().Put(context.Background(), content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if capturedMethod != http.MethodPut || capturedURL != "https://graph.microsoft.com/v1.0/drives/drive-id/items/root%3A%2F90%20Day%20Mentoring%20Challenge%2FTestimonial%20Library%2Fasset%20%231.mp4%3A/content" {
		t.Fatalf("captured request = %s %s", capturedMethod, capturedURL)
	}
	if capturedContentType != "application/octet-stream" || !bytes.Equal(capturedBody, content) {
		t.Fatalf("content type/body mismatch: %q, %q", capturedContentType, capturedBody)
	}
	receipt := &odUploadedItem{ID: deref(uploaded.GetId()), Name: deref(uploaded.GetName()), WebURL: deref(uploaded.GetWebUrl())}
	if err := validateOneDriveUploadedItem(receipt); err != nil {
		t.Fatalf("valid PUT receipt rejected: %v", err)
	}
}

type noOpGraphAuth struct{}

func (noOpGraphAuth) AuthenticateRequest(context.Context, *kiota.RequestInformation, map[string]interface{}) error {
	return nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
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
