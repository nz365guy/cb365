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

	kiota "github.com/microsoft/kiota-abstractions-go"
	khttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	drivesPkg "github.com/microsoftgraph/msgraph-sdk-go/drives"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
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

func TestOneDriveUploadSessionUsesDocumentedParentAndFilenameRoute(t *testing.T) {
	fileName := "90 day & story #1.mp4"
	got, err := oneDriveUploadSessionURL("https://graph.microsoft.com/v1.0", "drive-id", "parent!stable-id", fileName)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://graph.microsoft.com/v1.0/drives/drive-id/items/parent%21stable-id:/90%20day%20&%20story%20%231.mp4:/createUploadSession"
	if got != want {
		t.Fatalf("session URL = %q, want %q", got, want)
	}

	var capturedURL string
	var capturedMethod string
	var capturedContentType string
	var capturedBody []byte
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		capturedURL = request.URL.String()
		capturedMethod = request.Method
		capturedContentType = request.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(request.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"uploadUrl":"https://upload.example/session","expirationDateTime":"2026-10-13T00:00:00Z"}`)),
			Request:    request,
		}, nil
	})
	adapter, err := khttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		noOpGraphAuth{}, nil, nil, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	client := msgraphsdk.NewGraphServiceClient(adapter)
	client.BaseRequestBuilder.PathParameters["baseurl"] = "https://graph.microsoft.com/v1.0"
	item := models.NewDriveItemUploadableProperties()
	item.SetName(&fileName)
	item.GetAdditionalData()["@microsoft.graph.conflictBehavior"] = "fail"
	body := drivesPkg.NewItemItemsItemCreateUploadSessionPostRequestBody()
	body.SetItem(item)
	requestBuilder := client.Drives().ByDriveId("drive-id").Items().ByDriveItemId("parent!stable-id").CreateUploadSession().WithUrl(got)
	_, err = requestBuilder.Post(context.Background(), body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if capturedURL != want {
		t.Fatalf("SDK HTTP request URL = %q, want %q", capturedURL, want)
	}
	if capturedMethod != http.MethodPost || !strings.Contains(capturedContentType, "application/json") {
		t.Fatalf("SDK request method/content-type = %q/%q, want POST/application-json", capturedMethod, capturedContentType)
	}
	if !bytes.Contains(capturedBody, []byte(`"name":"90 day & story #1.mp4"`)) || !bytes.Contains(capturedBody, []byte(`"@microsoft.graph.conflictBehavior":"fail"`)) {
		t.Fatalf("SDK HTTP request body is missing expected name/conflictBehavior fields: %s", capturedBody)
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

func TestOneDriveParentPathUsesGraphSlashSeparators(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"video.mp4", "root"},
		{"Testimonial Library/video.mp4", "root:/Testimonial Library:"},
		{`Testimonial Library\\video.mp4`, "root:/Testimonial Library:"},
	} {
		if got := oneDriveParentPath(tc.input); got != tc.want {
			t.Errorf("oneDriveParentPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestOneDriveUploadSessionFailureShowsSafeRequestDiagnostics(t *testing.T) {
	filename := "report #1.mp4"
	const sessionURL = "https://graph.microsoft.com/v1.0/drives/secret-drive/items/secret-parent:/report%20%231.mp4:/createUploadSession"
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", request.Method)
		}
		responseBody := `{"error":{"code":"BadRequest","message":"do not echo this","innerError":{"request-id":"safe-request-id"}}}`
		return &http.Response{
			StatusCode:    http.StatusBadRequest,
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
	item := models.NewDriveItemUploadableProperties()
	item.SetName(&filename)
	item.GetAdditionalData()["@microsoft.graph.conflictBehavior"] = "fail"
	body := drivesPkg.NewItemItemsItemCreateUploadSessionPostRequestBody()
	body.SetItem(item)
	_, err = client.Drives().ByDriveId("secret-drive").Items().ByDriveItemId("secret-parent").CreateUploadSession().WithUrl(sessionURL).Post(context.Background(), body, nil)
	if err == nil {
		t.Fatal("expected a Graph error")
	}
	diagnostic := oneDriveUploadSessionFailure(err, sessionURL, "secret-drive", "secret-parent", filename, "fail").Error()
	for _, expected := range []string{"HTTP 400", "Graph code BadRequest", "request-id safe-request-id", "request=POST /v1.0/drives/{drive-id}/items/{parent-id}:/report%20%231.mp4:/createUploadSession", "Content-Type=application/json", "item.@microsoft.graph.conflictBehavior=\"fail\""} {
		if !strings.Contains(diagnostic, expected) {
			t.Errorf("diagnostic %q does not contain %q", diagnostic, expected)
		}
	}
	for _, secret := range []string{"secret-drive", "secret-parent", "do not echo this", "graph.microsoft.com"} {
		if strings.Contains(diagnostic, secret) {
			t.Errorf("diagnostic leaked %q: %s", secret, diagnostic)
		}
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
