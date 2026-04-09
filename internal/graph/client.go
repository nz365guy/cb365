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

// newHTTPClient creates an http.Client with the Graph SDK middleware pipeline.
func newHTTPClient(ipv4Only bool) *http.Client {
	clientOptions := msgraphsdk.GetDefaultClientOptions()
	middlewares := core.GetDefaultMiddlewaresWithOptions(&clientOptions)

	var httpClient *http.Client
	if ipv4Only {
		transport := khttp.NewCustomTransportWithParentTransport(NewIPv4Transport(), middlewares...)
		httpClient = &http.Client{Transport: transport}
	} else {
		transport := khttp.NewCustomTransport(middlewares...)
		httpClient = &http.Client{Transport: transport}
	}
	return httpClient
}

// NewGraphClient creates an authenticated msgraph-sdk-go client from a raw access token.
// Uses .default scope for the auth provider (fine for static tokens).
func NewGraphClient(accessToken string, expiresOn time.Time, ipv4Only bool) (*msgraphsdk.GraphServiceClient, error) {
	cred := &StaticTokenCredential{token: accessToken, expiresOn: expiresOn}
	return newGraphClientFromCredential(cred, []string{"https://graph.microsoft.com/.default"}, ipv4Only)
}

// NewGraphClientWithCredential creates an authenticated msgraph-sdk-go client from
// a live azcore.TokenCredential with explicit scopes. For MSAL-backed credentials,
// the scopes MUST match what was used during login so that MSAL can find cached tokens
// and silently refresh using the stored refresh token.
func NewGraphClientWithCredential(cred azcore.TokenCredential, scopes []string, ipv4Only bool) (*msgraphsdk.GraphServiceClient, error) {
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}
	return newGraphClientFromCredential(cred, scopes, ipv4Only)
}

func newGraphClientFromCredential(cred azcore.TokenCredential, scopes []string, ipv4Only bool) (*msgraphsdk.GraphServiceClient, error) {
	authProvider, err := azauth.NewAzureIdentityAuthenticationProviderWithScopes(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating auth provider: %w", err)
	}

	httpClient := newHTTPClient(ipv4Only)

	adapter, err := khttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		authProvider, nil, nil, httpClient,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request adapter: %w", err)
	}

	return msgraphsdk.NewGraphServiceClient(adapter), nil
}

