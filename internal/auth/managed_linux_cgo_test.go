//go:build linux && cgo

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nz365guy/cb365/internal/config"
)

func TestLegacyCachePermissionsAreCheckedBeforeMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	dir := filepath.Join(home, ".cache", ".IdentityService")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, legacyAzureIdentityCacheName)
	if err := os.WriteFile(file, []byte("encrypted fixture, never parsed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := verifyLegacyAzureIdentityCache(true); err != nil || got != file {
		t.Fatalf("valid legacy cache rejected: path=%q err=%v", got, err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := verifyLegacyAzureIdentityCache(true)
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheInvalid {
		t.Fatalf("expected invalid permission class, got %v", err)
	}
}

func TestOptionalLegacyCacheAllowsAlreadyDeletedPaths(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := verifyOptionalOwnedPath(missing, false, 0600, false); err != nil {
		t.Fatalf("optional missing path rejected: %v", err)
	}
	if err := verifyOptionalOwnedPath(missing, false, 0600, true); err == nil {
		t.Fatal("required missing path was accepted")
	}
}

func TestManagedProfileLockRejectsConcurrentWriter(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	first, err := acquireManagedProfileLock("profile", "host")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, err = acquireManagedProfileLock("profile", "host")
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheConflict {
		t.Fatalf("expected lock conflict, got %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	first = nil
	third, err := acquireManagedProfileLock("profile", "host")
	if err != nil {
		t.Fatalf("lock was not reclaimable: %v", err)
	}
	_ = third.Close()
}

func TestIncompleteManagedMigrationBlocksCredential(t *testing.T) {
	profile := &config.Profile{
		Name:     "profile",
		TenantID: "tenant",
		ClientID: "client",
		AuthMode: config.AuthModeDelegated,
		ManagedDelegated: &config.ManagedDelegatedMetadata{
			HomeAccountID:  "home",
			SecretID:       "secret",
			OrganisationID: "org",
			ProjectID:      "project",
			AssignedHost:   "host",
			MigrationState: managedMigrationCleanup,
		},
	}
	_, err := NewManagedDelegatedCredential(profile, false)
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheUnavailable {
		t.Fatalf("expected incomplete migration to fail closed, got %v", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal("managed error unexpectedly exposed an underlying filesystem error")
	}
}
