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
func NewGraphClient(accessToken string, expiresOn time.Time, ipv4Only bool) (*msgraphsdk.GraphServiceClient, error) {
	cred := &StaticTokenCredential{token: accessToken, expiresOn: expiresOn}

	scopes := []string{"https://graph.microsoft.com/.default"}
	authProvider, err := azauth.NewAzureIdentityAuthenticationProviderWithScopes(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating auth provider: %w", err)
	}

	var httpClient *http.Client
	if ipv4Only {
		httpClient = NewIPv4HTTPClient()
	} else {
		httpClient = http.DefaultClient
	}

	adapter, err := khttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		authProvider, nil, nil, httpClient,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request adapter: %w", err)
	}

	return msgraphsdk.NewGraphServiceClient(adapter), nil
}
