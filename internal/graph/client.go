package graph

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	azauth "github.com/microsoft/kiota-authentication-azure-go"
	khttp "github.com/microsoft/kiota-http-go"
	core "github.com/microsoftgraph/msgraph-sdk-go-core"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// StaticTokenCredential wraps a pre-acquired access token as an azcore.TokenCredential.
// SECURITY: The token is only held in memory and never logged.
type StaticTokenCredential struct {
	token     string
	expiresOn time.Time
}

// GetToken returns the stored access token.
func (s *StaticTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: s.token, ExpiresOn: s.expiresOn}, nil
}

// NewGraphClient creates an authenticated msgraph-sdk-go client from a raw access token.
// The caller is responsible for loading and validating the token.
//
// IMPORTANT: The HTTP client MUST include the Graph SDK middleware pipeline
// (telemetry, URL replacement for /me, retry, redirect). Without it, calls
// to client.Me() resolve to "/users/me-token-to-replace" which Graph rejects.
func NewGraphClient(accessToken string, expiresOn time.Time, ipv4Only bool) (*msgraphsdk.GraphServiceClient, error) {
	cred := &StaticTokenCredential{token: accessToken, expiresOn: expiresOn}

	scopes := []string{"https://graph.microsoft.com/.default"}
	authProvider, err := azauth.NewAzureIdentityAuthenticationProviderWithScopes(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating auth provider: %w", err)
	}

	// Build Graph middleware (includes URL replacement: /users/me-token-to-replace → /me)
	clientOptions := msgraphsdk.GetDefaultClientOptions()
	middlewares := core.GetDefaultMiddlewaresWithOptions(&clientOptions)

	var httpClient *http.Client
	if ipv4Only {
		// Wrap IPv4-only transport with Graph middleware pipeline
		transport := khttp.NewCustomTransportWithParentTransport(NewIPv4Transport(), middlewares...)
		httpClient = &http.Client{Transport: transport}
	} else {
		// Wrap default transport with Graph middleware pipeline
		transport := khttp.NewCustomTransport(middlewares...)
		httpClient = &http.Client{Transport: transport}
	}

	adapter, err := khttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		authProvider, nil, nil, httpClient,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request adapter: %w", err)
	}

	return msgraphsdk.NewGraphServiceClient(adapter), nil
}

