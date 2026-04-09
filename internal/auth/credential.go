package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azcache "github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/graph"
)

// msalCache is a persistent MSAL token cache shared across credential instances.
// It stores both access tokens (~60 min) and refresh tokens (~90 days) so that
// delegated credentials can silently renew without user interaction.
var msalCache azidentity.Cache

func init() {
	c, err := azcache.New(&azcache.Options{Name: "cb365"})
	if err != nil {
		// Cache will be zero-value; MSAL falls back to in-memory only.
		// This is non-fatal — the user just won't get persistent refresh tokens.
		return
	}
	msalCache = c
}

// DelegatedLoginResult holds the credential, token, and authentication record
// from an interactive delegated login. The AuthRecord must be persisted so that
// subsequent silent logins can look up the account in the MSAL cache.
type DelegatedLoginResult struct {
	Token      azcore.AccessToken
	AuthRecord azidentity.AuthenticationRecord
}

// LoginDelegatedWithCache performs a device-code login with the MSAL persistent cache.
// Returns the token AND the AuthenticationRecord, which MUST be stored for silent refresh.
func LoginDelegatedWithCache(ctx context.Context, profile *config.Profile, ipv4Only bool, promptFn func(context.Context, azidentity.DeviceCodeMessage) error) (*DelegatedLoginResult, error) {
	opts := &azidentity.DeviceCodeCredentialOptions{
		TenantID:   profile.TenantID,
		ClientID:   profile.ClientID,
		Cache:      msalCache,
		UserPrompt: promptFn,
	}

	if ipv4Only {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: graph.NewIPv4HTTPClient(),
		}
	}

	cred, err := azidentity.NewDeviceCodeCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("creating device code credential: %w", err)
	}

	scopes := GraphScopes(profile.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}

	// Authenticate() triggers device code flow AND returns the AuthenticationRecord
	// needed for future silent token acquisitions from the cache.
	record, err := cred.Authenticate(ctx, &policy.TokenRequestOptions{Scopes: scopes, EnableCAE: true})
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Now get the actual token (should be instant — MSAL just cached it)
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: scopes, EnableCAE: true})
	if err != nil {
		return nil, fmt.Errorf("acquiring token after auth: %w", err)
	}

	return &DelegatedLoginResult{Token: token, AuthRecord: record}, nil
}

// NewDelegatedCredentialSilent creates a DeviceCodeCredential backed by the
// persistent MSAL cache with a stored AuthenticationRecord. The UserPrompt
// returns an error so that if MSAL can't silently refresh (refresh token expired),
// it fails fast rather than printing a device code in an unattended context.
//
// The authRecordJSON must be the JSON from a previous LoginDelegatedWithCache call.
func NewDelegatedCredentialSilent(profile *config.Profile, authRecordJSON string, ipv4Only bool) (azcore.TokenCredential, error) {
	if authRecordJSON == "" {
		return nil, fmt.Errorf("no authentication record stored — run 'cb365 auth login --profile %s' to authenticate", profile.Name)
	}

	var record azidentity.AuthenticationRecord
	if err := json.Unmarshal([]byte(authRecordJSON), &record); err != nil {
		return nil, fmt.Errorf("parsing stored authentication record: %w", err)
	}

	opts := &azidentity.DeviceCodeCredentialOptions{
		TenantID:             profile.TenantID,
		ClientID:             profile.ClientID,
		Cache:                msalCache,
		AuthenticationRecord: record,
		UserPrompt: func(_ context.Context, _ azidentity.DeviceCodeMessage) error {
			return fmt.Errorf("interactive login required — run 'cb365 auth login --profile %s'", profile.Name)
		},
	}

	if ipv4Only {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: graph.NewIPv4HTTPClient(),
		}
	}

	cred, err := azidentity.NewDeviceCodeCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("creating silent credential: %w", err)
	}
	return cred, nil
}

// GetTokenSilent attempts a silent token acquisition for a delegated profile.
// Returns an access token if the MSAL cache has a valid refresh token, or an
// error if interactive login is required.
func GetTokenSilent(ctx context.Context, profile *config.Profile, authRecordJSON string, scopes []string, ipv4Only bool) (azcore.AccessToken, error) {
	cred, err := NewDelegatedCredentialSilent(profile, authRecordJSON, ipv4Only)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	fullScopes := GraphScopes(scopes)
	if len(fullScopes) == 0 {
		fullScopes = []string{"https://graph.microsoft.com/.default"}
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: fullScopes, EnableCAE: true})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("silent token refresh failed — run 'cb365 auth login --profile %s' to re-authenticate: %w", profile.Name, err)
	}
	return token, nil
}

