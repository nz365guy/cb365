package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nz365guy/cb365/internal/config"
	"golang.org/x/crypto/pbkdf2"
)

const (
	fileStoreName          = "tokens.enc"
	legacyFileStoreVersion = 1
	fileStoreVersion       = 2
	pbkdf2Iter             = 210_000 // OWASP 2023 recommendation for SHA-256
	pbkdf2KeyLen           = 32      // AES-256
	saltLen                = 16
)

// fileBackend stores encrypted tokens in ~/.config/cb365/tokens.enc
// Format: JSON envelope with salt + per-profile AES-256-GCM encrypted entries
type fileBackend struct {
	mu       sync.Mutex
	path     string
	password string
}

// encryptedStore is the on-disk format
type encryptedStore struct {
	Version int               `json:"version"`
	Salt    []byte            `json:"salt"`    // PBKDF2 salt (hex would also work but base64 via json is fine)
	Entries map[string][]byte `json:"entries"` // profile → AES-256-GCM ciphertext (nonce prepended)
}

// newFileStore creates an encrypted file token store.
// Requires CB365_KEYRING_PASSWORD to be set.
func newFileStore() (*fileBackend, error) {
	password := os.Getenv("CB365_KEYRING_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("CB365_KEYRING_PASSWORD not set")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	path := filepath.Join(home, ".config", "cb365", fileStoreName)
	return &fileBackend{path: path, password: password}, nil
}

func (f *fileBackend) load() (*encryptedStore, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			// New store — generate fresh salt
			salt := make([]byte, saltLen)
			if _, err := io.ReadFull(rand.Reader, salt); err != nil {
				return nil, fmt.Errorf("generating salt: %w", err)
			}
			return &encryptedStore{
				Version: fileStoreVersion,
				Salt:    salt,
				Entries: make(map[string][]byte),
			}, nil
		}
		return nil, fmt.Errorf("reading token store: %w", err)
	}

	var store encryptedStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing token store: %w", err)
	}
	if store.Entries == nil {
		store.Entries = make(map[string][]byte)
	}
	return &store, nil
}

func (f *fileBackend) save(store *encryptedStore) error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling token store: %w", err)
	}

	// Write atomically via temp file
	tmpFile, err := os.CreateTemp(dir, ".tokens-*.tmp")
	if err != nil {
		return fmt.Errorf("creating token store temporary file: %w", err)
	}
	tmp := tmpFile.Name()
	defer os.Remove(tmp) // #nosec G104 -- best-effort cleanup
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("restricting token store temporary file: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing token store: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("syncing token store: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing token store: %w", err)
	}
	if err := replaceFile(tmp, f.path); err != nil {
		_ = os.Remove(tmp) // #nosec G104 — best-effort cleanup on failed rename
		return fmt.Errorf("replacing token store: %w", err)
	}
	return nil
}

func (f *fileBackend) deriveKey(salt []byte) []byte {
	return pbkdf2.Key([]byte(f.password), salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
}

func (f *fileBackend) encrypt(key, plaintext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Prepend nonce to ciphertext
	return gcm.Seal(nonce, nonce, plaintext, additionalData), nil
}

func (f *fileBackend) decrypt(key, ciphertext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, additionalData)
}

// --- tokenStore interface ---

func (f *fileBackend) Set(profile string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		return err
	}

	if store.Version != fileStoreVersion && len(store.Entries) != 0 {
		return fmt.Errorf("legacy token store is not profile-bound; run 'cb365 auth migrate --profile <app-only-profile>'")
	}
	store.Version = fileStoreVersion
	key := f.deriveKey(store.Salt)
	encrypted, err := f.encrypt(key, data, []byte(profile))
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	store.Entries[profile] = encrypted
	return f.save(store)
}

func (f *fileBackend) Get(profile string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		return nil, err
	}

	encrypted, ok := store.Entries[profile]
	if !ok {
		return nil, errNotFound
	}

	if store.Version != fileStoreVersion {
		return nil, fmt.Errorf("legacy token store is not profile-bound; run 'cb365 auth migrate --profile %s'", profile)
	}
	key := f.deriveKey(store.Salt)
	return f.decrypt(key, encrypted, []byte(profile))
}

func (f *fileBackend) Delete(profile string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		return err
	}

	if _, ok := store.Entries[profile]; !ok {
		return nil // already gone
	}

	delete(store.Entries, profile)
	return f.save(store)
}

func (f *fileBackend) migrateLegacyAppOnlyTokens(profiles map[string]*config.Profile, selectedProfile string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		return 0, err
	}
	if store.Version == fileStoreVersion {
		return 0, fmt.Errorf("app-only token store is already profile-bound")
	}
	// Accept both the legacy versioned format (1) and the pre-versioning
	// format (0): both store entries encrypted without profile binding.
	if store.Version != legacyFileStoreVersion && store.Version != 0 {
		return 0, fmt.Errorf("unsupported app-only token store version %d; reauthentication is required", store.Version)
	}
	if len(store.Entries) == 0 {
		return 0, fmt.Errorf("legacy app-only token store has no entries")
	}
	if _, ok := store.Entries[selectedProfile]; !ok {
		return 0, fmt.Errorf("selected app-only profile %q has no legacy credential entry", selectedProfile)
	}

	key := f.deriveKey(store.Salt)
	defer zeroBytes(key)
	boundEntries := make(map[string][]byte, len(store.Entries))
	seenBindings := make(map[string]string, len(store.Entries))

	for profileName, ciphertext := range store.Entries {
		profile, ok := profiles[profileName]
		if !ok || profile == nil {
			return 0, fmt.Errorf("legacy credential entry %q has no configured profile; reauthentication is required", profileName)
		}
		if profile.Name != profileName || profile.AuthMode != config.AuthModeAppOnly || profile.TenantID == "" || profile.ClientID == "" {
			return 0, fmt.Errorf("legacy credential entry %q is not a complete app-only profile", profileName)
		}

		plaintext, decryptErr := f.decrypt(key, ciphertext, nil)
		if decryptErr != nil {
			return 0, fmt.Errorf("decrypting legacy credential entry %q: %w", profileName, decryptErr)
		}
		var cache TokenCache
		if unmarshalErr := json.Unmarshal(plaintext, &cache); unmarshalErr != nil {
			zeroBytes(plaintext)
			return 0, fmt.Errorf("parsing legacy credential entry %q: %w", profileName, unmarshalErr)
		}

		tenantID, clientID, claimsErr := legacyTokenBindingClaims(cache.AccessToken)
		if claimsErr != nil {
			zeroBytes(plaintext)
			return 0, fmt.Errorf("validating legacy credential entry %q: %w", profileName, claimsErr)
		}
		if !strings.EqualFold(tenantID, profile.TenantID) || !strings.EqualFold(clientID, profile.ClientID) {
			zeroBytes(plaintext)
			return 0, fmt.Errorf("legacy credential entry %q does not match its configured tenant and client", profileName)
		}

		credentialKind, kindErr := validateLegacyCredentialKind(&cache)
		if kindErr != nil {
			zeroBytes(plaintext)
			return 0, fmt.Errorf("validating legacy credential entry %q: %w", profileName, kindErr)
		}
		binding := strings.ToLower(profile.TenantID + "\x00" + profile.ClientID + "\x00" + credentialKind)
		if prior, duplicate := seenBindings[binding]; duplicate {
			zeroBytes(plaintext)
			return 0, fmt.Errorf("legacy profiles %q and %q have indistinguishable credential bindings; reauthentication is required", prior, profileName)
		}
		seenBindings[binding] = profileName

		bound, encryptErr := f.encrypt(key, plaintext, []byte(profileName))
		zeroBytes(plaintext)
		if encryptErr != nil {
			return 0, fmt.Errorf("binding legacy credential entry %q: %w", profileName, encryptErr)
		}
		boundEntries[profileName] = bound
	}

	migrated := &encryptedStore{Version: fileStoreVersion, Salt: store.Salt, Entries: boundEntries}
	if err := f.save(migrated); err != nil {
		return 0, fmt.Errorf("saving profile-bound token store: %w", err)
	}
	return len(boundEntries), nil
}

func legacyTokenBindingClaims(accessToken string) (string, string, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("access token has invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("decoding access token claims: %w", err)
	}
	var claims struct {
		TenantID string `json:"tid"`
		AppID    string `json:"appid"`
		AZP      string `json:"azp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", fmt.Errorf("parsing access token claims: %w", err)
	}
	clientID := claims.AppID
	if clientID == "" {
		clientID = claims.AZP
	}
	if claims.TenantID == "" || clientID == "" {
		return "", "", fmt.Errorf("access token lacks tenant or client binding claims")
	}
	return claims.TenantID, clientID, nil
}

func validateLegacyCredentialKind(cache *TokenCache) (string, error) {
	hasCertificate := cache.CertPath != ""
	hasClientSecret := cache.ClientSecret != ""
	if hasCertificate == hasClientSecret {
		return "", fmt.Errorf("entry must contain exactly one refresh credential")
	}
	if hasClientSecret {
		return "client-secret", nil
	}
	info, err := os.Stat(cache.CertPath)
	if err != nil {
		return "", fmt.Errorf("certificate path is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("certificate path is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("certificate file permissions are too broad; require 0600 or stricter")
	}
	return "certificate", nil
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
