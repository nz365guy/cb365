package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	fileStoreName   = "tokens.enc"
	pbkdf2Iter      = 210_000 // OWASP 2023 recommendation for SHA-256
	pbkdf2KeyLen    = 32      // AES-256
	saltLen         = 16
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
	Salt    []byte            `json:"salt"`     // PBKDF2 salt (hex would also work but base64 via json is fine)
	Entries map[string][]byte `json:"entries"`  // profile → AES-256-GCM ciphertext (nonce prepended)
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
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing token store: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		_ = os.Remove(tmp) // #nosec G104 — best-effort cleanup on failed rename
		return fmt.Errorf("replacing token store: %w", err)
	}
	return nil
}

func (f *fileBackend) deriveKey(salt []byte) []byte {
	return pbkdf2.Key([]byte(f.password), salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)
}

func (f *fileBackend) encrypt(key, plaintext []byte) ([]byte, error) {
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
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (f *fileBackend) decrypt(key, ciphertext []byte) ([]byte, error) {
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
	return gcm.Open(nil, nonce, ct, nil)
}

// --- tokenStore interface ---

func (f *fileBackend) Set(profile string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, err := f.load()
	if err != nil {
		return err
	}

	key := f.deriveKey(store.Salt)
	encrypted, err := f.encrypt(key, data)
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

	key := f.deriveKey(store.Salt)
	return f.decrypt(key, encrypted)
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

