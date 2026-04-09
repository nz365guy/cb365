package auth

import (
	"context"
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

// NewDelegatedCredentialInteractive creates a DeviceCodeCredential with the
// persistent MSAL cache and an interactive user prompt. Use this for login.
func NewDelegatedCredentialInteractive(profile *config.Profile, ipv4Only bool, promptFn func(context.Context, azidentity.DeviceCodeMessage) error) (azcore.TokenCredential, error) {
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
	return cred, nil
}

// NewDelegatedCredentialSilent creates a DeviceCodeCredential backed by the
// persistent MSAL cache. The UserPrompt returns an error so that if MSAL
// can't silently refresh (refresh token expired), it fails fast rather than
// printing a device code to stdout in an unattended context.
//
// On success, GetToken() silently uses the cached refresh token.
// On failure, the caller should instruct the user to run 'cb365 auth login'.
func NewDelegatedCredentialSilent(profile *config.Profile, ipv4Only bool) (azcore.TokenCredential, error) {
	opts := &azidentity.DeviceCodeCredentialOptions{
		TenantID: profile.TenantID,
		ClientID: profile.ClientID,
		Cache:    msalCache,
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
func GetTokenSilent(ctx context.Context, profile *config.Profile, scopes []string, ipv4Only bool) (azcore.AccessToken, error) {
	cred, err := NewDelegatedCredentialSilent(profile, ipv4Only)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	fullScopes := GraphScopes(scopes)
	if len(fullScopes) == 0 {
		fullScopes = []string{"https://graph.microsoft.com/.default"}
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: fullScopes})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("silent token refresh failed — run 'cb365 auth login --profile %s' to re-authenticate: %w", profile.Name, err)
	}
	return token, nil
}

