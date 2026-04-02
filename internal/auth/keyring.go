package auth

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "cb365"
)

// TokenCache represents cached authentication data
// SECURITY: Never log or print this struct — it contains secrets
type TokenCache struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// StoreToken securely stores a token in the OS keychain
func StoreToken(profileName string, cache *TokenCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshalling token cache: %w", err)
	}
	if err := keyring.Set(keyringService, profileName, string(data)); err != nil {
		return fmt.Errorf("storing token in keychain: %w", err)
	}
	return nil
}

// LoadToken retrieves a token from the OS keychain
func LoadToken(profileName string) (*TokenCache, error) {
	data, err := keyring.Get(keyringService, profileName)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, fmt.Errorf("no cached token for profile %q — run 'cb365 auth login --profile %s'", profileName, profileName)
		}
		return nil, fmt.Errorf("reading token from keychain: %w", err)
	}

	var cache TokenCache
	if err := json.Unmarshal([]byte(data), &cache); err != nil {
		return nil, fmt.Errorf("parsing cached token: %w", err)
	}
	return &cache, nil
}

// DeleteToken removes a token from the OS keychain
func DeleteToken(profileName string) error {
	if err := keyring.Delete(keyringService, profileName); err != nil {
		if err == keyring.ErrNotFound {
			return nil // already gone
		}
		return fmt.Errorf("deleting token from keychain: %w", err)
	}
	return nil
}
