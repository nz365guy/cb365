//go:build linux && cgo

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/nz365guy/cb365/internal/config"
	"golang.org/x/sys/unix"
)

const legacyAzureIdentityCacheName = "cb365"

// legacyDelegatedState contains only the non-secret account binding and the
// already-verified locations that must be removed after the BWS record has
// passed its write/readback checks. It never retains a legacy token cache.
type legacyDelegatedState struct {
	profileName   string
	homeAccountID string
}

func inspectLegacyDelegated(profile *config.Profile) (*legacyDelegatedState, error) {
	if profile == nil || profile.Name == "" || profile.AuthMode != config.AuthModeDelegated || profile.ManagedDelegated != nil {
		return nil, managedError(ManagedCacheInvalid, "inspect legacy delegated profile", nil)
	}

	store, err := getStore()
	if err != nil {
		return nil, managedError(ManagedCacheUnavailable, "open legacy delegated token store", err)
	}
	// G4: validate every filesystem-backed legacy layer before reading any
	// bearer-bearing data. OS keyring entries are isolated by the current user
	// session and have no POSIX mode bits to inspect.
	if err := verifyLegacyTokenStorePermissions(store, true); err != nil {
		return nil, err
	}
	if _, err := verifyLegacyAzureIdentityCache(true); err != nil {
		return nil, err
	}

	data, err := store.Get(profile.Name)
	if err != nil {
		return nil, managedError(ManagedCacheUnavailable, "read legacy delegated token metadata", err)
	}
	var token TokenCache
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, managedError(ManagedCacheInvalid, "parse legacy delegated token metadata", err)
	}
	if token.AuthRecord == "" {
		return nil, managedError(ManagedCacheInvalid, "validate legacy delegated authentication record", nil)
	}
	var record azidentity.AuthenticationRecord
	if err := json.Unmarshal([]byte(token.AuthRecord), &record); err != nil {
		return nil, managedError(ManagedCacheInvalid, "parse legacy delegated authentication record", err)
	}
	if record.HomeAccountID == "" {
		return nil, managedError(ManagedCacheInvalid, "validate legacy delegated account binding", nil)
	}

	return &legacyDelegatedState{profileName: profile.Name, homeAccountID: record.HomeAccountID}, nil
}

func (s *legacyDelegatedState) cleanupAndVerify() error {
	if s == nil || s.profileName == "" {
		return managedError(ManagedCacheInvalid, "clean up legacy delegated credential", nil)
	}
	return cleanupLegacyDelegated(s.profileName)
}

func cleanupLegacyDelegated(profileName string) error {
	if profileName == "" {
		return managedError(ManagedCacheInvalid, "clean up legacy delegated credential", nil)
	}
	store, err := getStore()
	if err != nil {
		return managedError(ManagedCacheUnavailable, "open legacy delegated token store", err)
	}
	if err := verifyLegacyTokenStorePermissions(store, false); err != nil {
		return err
	}
	if err := store.Delete(profileName); err != nil {
		return managedError(ManagedCacheUnavailable, "delete legacy delegated token metadata", err)
	}
	if _, err := store.Get(profileName); !errors.Is(err, errNotFound) {
		return managedError(ManagedCacheConflict, "verify legacy delegated token deletion", nil)
	}

	cacheFile, err := verifyLegacyAzureIdentityCache(false)
	if err != nil {
		return err
	}
	if cacheFile != "" {
		if err := os.Remove(cacheFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache file", err)
		}
		if _, err := os.Lstat(cacheFile); !errors.Is(err, os.ErrNotExist) {
			return managedError(ManagedCacheConflict, "verify legacy Azure Identity cache file deletion", nil)
		}
	}
	if err := deleteLegacyAzureIdentityKey(); err != nil {
		return err
	}
	return nil
}

func verifyLegacyTokenStorePermissions(store tokenStore, required bool) error {
	fileStore, ok := store.(*fileBackend)
	if !ok {
		// The OS credential service scopes entries to the effective user. It has
		// no caller-visible filesystem owner or mode to validate.
		return nil
	}
	dir := filepath.Dir(fileStore.path)
	if err := verifyOptionalOwnedPath(dir, true, 0700, required); err != nil {
		return err
	}
	return verifyOptionalOwnedPath(fileStore.path, false, 0600, required)
}

// verifyLegacyAzureIdentityCache returns the deterministic encrypted cache
// path used by azidentity/cache v0.4.0 on Linux. The cache is never opened or
// parsed here; its owner and modes are checked before any migration read.
func verifyLegacyAzureIdentityCache(required bool) (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", managedError(ManagedCacheUnavailable, "resolve legacy Azure Identity cache directory", err)
		}
		base = filepath.Join(home, ".cache")
	}
	if !filepath.IsAbs(base) {
		return "", managedError(ManagedCacheInvalid, "validate legacy Azure Identity cache directory", nil)
	}
	dir := filepath.Join(base, ".IdentityService")
	file := filepath.Join(dir, legacyAzureIdentityCacheName)
	if err := verifyOptionalOwnedPath(dir, true, 0700, required); err != nil {
		return "", err
	}
	if err := verifyOptionalOwnedPath(file, false, 0600, required); err != nil {
		return "", err
	}
	return file, nil
}

func verifyOptionalOwnedPath(path string, directory bool, mode os.FileMode, required bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return managedError(ManagedCacheUnavailable, "inspect legacy delegated cache path", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || info.Mode().Perm() != mode {
		return managedError(ManagedCacheInvalid, "validate legacy delegated cache path permissions", nil)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return managedError(ManagedCacheInvalid, "validate legacy delegated cache path ownership", nil)
	}
	return nil
}

func deleteLegacyAzureIdentityKey() error {
	userRing, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_USER_KEYRING, false)
	if err != nil {
		if isLegacyKeyAbsent(err) {
			return nil
		}
		return managedError(ManagedCacheUnavailable, "open legacy Azure Identity keyring", err)
	}
	rings := []int{userRing}
	if persistentRing, persistentErr := unix.KeyctlInt(unix.KEYCTL_GET_PERSISTENT, -1, userRing, 0, 0); persistentErr == nil && persistentRing != userRing {
		rings = append(rings, persistentRing)
	}
	for _, ring := range rings {
		for {
			keyID, searchErr := unix.KeyctlSearch(ring, "user", legacyAzureIdentityCacheName, 0)
			if searchErr != nil {
				if isLegacyKeyAbsent(searchErr) {
					break
				}
				return managedError(ManagedCacheUnavailable, "locate legacy Azure Identity cache key", searchErr)
			}
			if _, unlinkErr := unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, ring, 0, 0); unlinkErr != nil && !isLegacyKeyAbsent(unlinkErr) {
				return managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache key", unlinkErr)
			}
		}
		if _, searchErr := unix.KeyctlSearch(ring, "user", legacyAzureIdentityCacheName, 0); !isLegacyKeyAbsent(searchErr) {
			return managedError(ManagedCacheConflict, "verify legacy Azure Identity cache key deletion", nil)
		}
	}
	return nil
}

func isLegacyKeyAbsent(err error) bool {
	return errors.Is(err, unix.ENOKEY) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EKEYEXPIRED) || errors.Is(err, unix.EKEYREVOKED)
}
