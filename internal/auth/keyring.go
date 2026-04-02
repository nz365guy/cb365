package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

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
	ClientSecret string `json:"client_secret,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// tokenStore abstracts token storage backends
type tokenStore interface {
	Set(profile string, data []byte) error
	Get(profile string) ([]byte, error)
	Delete(profile string) error
}

var (
	storeOnce     sync.Once
	activeStore   tokenStore
	storeInitErr  error
)

// getStore returns the active token store, initializing on first call.
// Selection order:
//  1. CB365_TOKEN_STORE=file → encrypted file (requires CB365_KEYRING_PASSWORD)
//  2. OS keyring (go-keyring) → try a probe write/read/delete
//  3. Fallback to encrypted file (requires CB365_KEYRING_PASSWORD)
//  4. Error with clear instructions
func getStore() (tokenStore, error) {
	storeOnce.Do(func() {
		// Explicit override
		if os.Getenv("CB365_TOKEN_STORE") == "file" {
			activeStore, storeInitErr = newFileStore()
			return
		}

		// Try OS keyring with a probe
		if probeKeyring() {
			activeStore = &keyringBackend{}
			return
		}

		// Fallback to encrypted file
		activeStore, storeInitErr = newFileStore()
		if storeInitErr != nil {
			storeInitErr = fmt.Errorf(
				"OS keychain unavailable (no D-Bus/secret-service) and encrypted file fallback not configured.\n"+
					"Set CB365_KEYRING_PASSWORD to a strong passphrase to enable encrypted file storage.\n"+
					"Original error: %w", storeInitErr)
		}
	})
	return activeStore, storeInitErr
}

// probeKeyring tests if the OS keyring is functional
func probeKeyring() bool {
	const probeKey = "__cb365_probe__"
	if err := keyring.Set(keyringService, probeKey, "probe"); err != nil {
		return false
	}
	_, err := keyring.Get(keyringService, probeKey)
	if err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, probeKey)
	return true
}

// --- OS keyring backend ---

type keyringBackend struct{}

func (k *keyringBackend) Set(profile string, data []byte) error {
	return keyring.Set(keyringService, profile, string(data))
}

func (k *keyringBackend) Get(profile string) ([]byte, error) {
	val, err := keyring.Get(keyringService, profile)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, errNotFound
		}
		return nil, err
	}
	return []byte(val), nil
}

func (k *keyringBackend) Delete(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

// sentinel for "not found" across backends
var errNotFound = fmt.Errorf("token not found")

// --- Public API (unchanged) ---

// StoreToken securely stores a token
func StoreToken(profileName string, cache *TokenCache) error {
	store, err := getStore()
	if err != nil {
		return fmt.Errorf("initializing token store: %w", err)
	}
	data, err := json.Marshal(cache) // #nosec G117 — intentional: serializing token for encrypted storage
	if err != nil {
		return fmt.Errorf("marshalling token cache: %w", err)
	}
	if err := store.Set(profileName, data); err != nil {
		return fmt.Errorf("storing token: %w", err)
	}
	return nil
}

// LoadToken retrieves a token
func LoadToken(profileName string) (*TokenCache, error) {
	store, err := getStore()
	if err != nil {
		return nil, fmt.Errorf("initializing token store: %w", err)
	}
	data, err := store.Get(profileName)
	if err != nil {
		if err == errNotFound {
			return nil, fmt.Errorf("no cached token for profile %q — run 'cb365 auth login --profile %s'", profileName, profileName)
		}
		return nil, fmt.Errorf("reading token: %w", err)
	}
	var cache TokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parsing cached token: %w", err)
	}
	return &cache, nil
}

// DeleteToken removes a token
func DeleteToken(profileName string) error {
	store, err := getStore()
	if err != nil {
		return fmt.Errorf("initializing token store: %w", err)
	}
	return store.Delete(profileName)
}

