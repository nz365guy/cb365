package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nz365guy/cb365/internal/config"
)

func TestFileStoreBindsCiphertextToProfile(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	want := []byte(`{"access_token":"synthetic"}`)
	if err := backend.Set("profile-a", want); err != nil {
		t.Fatal(err)
	}
	got, err := backend.Get("profile-a")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("round trip failed: got %q, err %v", got, err)
	}

	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Entries["profile-b"] = append([]byte(nil), store.Entries["profile-a"]...)
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get("profile-b"); err == nil {
		t.Fatal("ciphertext moved to another profile label must not decrypt")
	}
}

func TestFileStoreRejectsLegacyUnboundEntries(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Version = 0
	store.Entries["legacy"] = []byte("synthetic-ciphertext")
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get("legacy"); err == nil {
		t.Fatal("legacy unbound entry must require reauthentication")
	}
}

func TestFileStoreMigratesLegacyAppOnlyEntriesAtomically(t *testing.T) {
	dir := t.TempDir()
	backend := &fileBackend{path: filepath.Join(dir, fileStoreName), password: "synthetic-test-password"}
	certificatePath := filepath.Join(dir, "certificate.pem")
	if err := os.WriteFile(certificatePath, []byte("synthetic certificate fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles := map[string]*config.Profile{
		"cert-profile": {
			Name: "cert-profile", TenantID: "tenant-a", ClientID: "client-a", AuthMode: config.AuthModeAppOnly,
		},
		"secret-profile": {
			Name: "secret-profile", TenantID: "tenant-a", ClientID: "client-b", AuthMode: config.AuthModeAppOnly,
		},
	}
	writeLegacyEntry(t, backend, "cert-profile", TokenCache{
		AccessToken: syntheticAppToken(t, "tenant-a", "client-a"), CertPath: certificatePath,
	})
	writeLegacyEntry(t, backend, "secret-profile", TokenCache{
		AccessToken: syntheticAppToken(t, "tenant-a", "client-b"), ClientSecret: "synthetic-placeholder",
	})

	count, err := backend.migrateLegacyAppOnlyTokens(profiles, "cert-profile")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migrated %d entries, want 2", count)
	}
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	if store.Version != fileStoreVersion {
		t.Fatalf("store version = %d, want %d", store.Version, fileStoreVersion)
	}
	for name := range profiles {
		got, err := backend.Get(name)
		if err != nil {
			t.Fatalf("profile-bound read for %q failed: %v", name, err)
		}
		var cache TokenCache
		if err := json.Unmarshal(got, &cache); err != nil {
			t.Fatal(err)
		}
		if cache.AccessToken == "" {
			t.Fatalf("profile %q lost its token", name)
		}
		zeroBytes(got)
	}
}

func TestFileStoreLegacyMigrationRejectsMismatchedBindingWithoutWriting(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	writeLegacyEntry(t, backend, "profile-a", TokenCache{
		AccessToken: syntheticAppToken(t, "tenant-a", "unexpected-client"), ClientSecret: "synthetic-placeholder",
	})
	before, err := os.ReadFile(backend.path)
	if err != nil {
		t.Fatal(err)
	}

	profiles := map[string]*config.Profile{
		"profile-a": {
			Name: "profile-a", TenantID: "tenant-a", ClientID: "client-a", AuthMode: config.AuthModeAppOnly,
		},
	}
	if _, err := backend.migrateLegacyAppOnlyTokens(profiles, "profile-a"); err == nil {
		t.Fatal("mismatched client binding should fail")
	}
	after, err := os.ReadFile(backend.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed migration modified the legacy store")
	}
}

func TestFileStoreMigratesUnversionedLegacyStore(t *testing.T) {
	dir := t.TempDir()
	backend := &fileBackend{path: filepath.Join(dir, fileStoreName), password: "synthetic-test-password"}

	profiles := map[string]*config.Profile{
		"work-cert": {
			Name: "work-cert", TenantID: "tenant-a", ClientID: "client-a", AuthMode: config.AuthModeAppOnly,
		},
	}
	writeLegacyEntryAtVersion(t, backend, 0, "work-cert", TokenCache{
		AccessToken: syntheticAppToken(t, "tenant-a", "client-a"), ClientSecret: "synthetic-placeholder",
	})

	count, err := backend.migrateLegacyAppOnlyTokens(profiles, "work-cert")
	if err != nil {
		t.Fatalf("v0 (unversioned) store migration failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("migrated %d entries, want 1", count)
	}
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	if store.Version != fileStoreVersion {
		t.Fatalf("store version = %d, want %d", store.Version, fileStoreVersion)
	}
}

func TestFileStoreLegacyMigrationRejectsUnknownVersionWithoutWriting(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	writeLegacyEntry(t, backend, "profile-a", TokenCache{
		AccessToken: syntheticAppToken(t, "tenant-a", "client-a"), ClientSecret: "synthetic-placeholder",
	})
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Version = fileStoreVersion + 1
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(backend.path)
	if err != nil {
		t.Fatal(err)
	}

	profiles := map[string]*config.Profile{
		"profile-a": {
			Name: "profile-a", TenantID: "tenant-a", ClientID: "client-a", AuthMode: config.AuthModeAppOnly,
		},
	}
	if _, err := backend.migrateLegacyAppOnlyTokens(profiles, "profile-a"); err == nil {
		t.Fatal("unknown store version should fail")
	}
	after, err := os.ReadFile(backend.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed migration modified the unknown-version store")
	}
}

func writeLegacyEntry(t *testing.T, backend *fileBackend, profile string, cache TokenCache) {
	t.Helper()
	writeLegacyEntryAtVersion(t, backend, legacyFileStoreVersion, profile, cache)
}

func writeLegacyEntryAtVersion(t *testing.T, backend *fileBackend, version int, profile string, cache TokenCache) {
	t.Helper()
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Version = version
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	key := backend.deriveKey(store.Salt)
	ciphertext, err := backend.encrypt(key, data, nil)
	zeroBytes(key)
	zeroBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	store.Entries[profile] = ciphertext
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
}

func syntheticAppToken(t *testing.T, tenantID, clientID string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"tid": tenantID, "appid": clientID})
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
