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

var legacyAzureIdentityCacheNames = []string{"cb365", "cb365.cae"}

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
	cacheFiles, err := verifyLegacyAzureIdentityCache(true)
	if err != nil {
		return nil, err
	}
	locks, err := acquireLegacyAzureIdentityLocks(cacheFiles)
	if err != nil {
		return nil, err
	}
	if err := releaseLegacyAzureIdentityLocks(locks, true); err != nil {
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

	cacheFiles, err := verifyLegacyAzureIdentityCache(false)
	if err != nil {
		return err
	}
	locks, err := acquireLegacyAzureIdentityLocks(cacheFiles)
	if err != nil {
		return err
	}
	defer func() { _ = releaseLegacyAzureIdentityLocks(locks, false) }()
	for _, cacheFile := range cacheFiles {
		if err := os.Remove(cacheFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache file", err)
		}
		if _, err := os.Lstat(cacheFile); !errors.Is(err, os.ErrNotExist) {
			return managedError(ManagedCacheConflict, "verify legacy Azure Identity cache file deletion", nil)
		}
	}
	if err := releaseLegacyAzureIdentityLocks(locks, true); err != nil {
		return err
	}
	locks = nil
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
func verifyLegacyAzureIdentityCache(required bool) ([]string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, managedError(ManagedCacheUnavailable, "resolve legacy Azure Identity cache directory", err)
		}
		base = filepath.Join(home, ".cache")
	}
	if !filepath.IsAbs(base) {
		return nil, managedError(ManagedCacheInvalid, "validate legacy Azure Identity cache directory", nil)
	}
	dir := filepath.Join(base, ".IdentityService")
	if err := verifyOptionalOwnedPath(dir, true, 0700, required); err != nil {
		return nil, err
	}
	files := make([]string, 0, len(legacyAzureIdentityCacheNames))
	found := 0
	for _, name := range legacyAzureIdentityCacheNames {
		file := filepath.Join(dir, name)
		if _, err := os.Lstat(file); err == nil { // #nosec G703 -- dir is absolute and ownership-checked; name comes from a fixed internal allowlist
			found++
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, managedError(ManagedCacheUnavailable, "inspect legacy delegated cache path", err)
		}
		if err := verifyOptionalOwnedPath(file, false, 0600, false); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if required && found == 0 {
		return nil, managedError(ManagedCacheUnavailable, "locate legacy Azure Identity cache", os.ErrNotExist)
	}
	return files, nil
}

func acquireLegacyAzureIdentityLocks(cacheFiles []string) ([]*os.File, error) {
	if len(cacheFiles) == 0 {
		return nil, nil
	}
	dir := filepath.Dir(cacheFiles[0])
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, managedError(ManagedCacheUnavailable, "inspect legacy Azure Identity lock directory", err)
	}
	locks := make([]*os.File, 0, len(cacheFiles))
	for _, cacheFile := range cacheFiles {
		lockPath := cacheFile + ".lockfile"
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_CLOEXEC, 0600) // #nosec G304 -- derived from verified cache directory
		if err != nil {
			primary := managedError(ManagedCacheUnavailable, "open legacy Azure Identity cache lock", err)
			return nil, cleanupLegacyAzureIdentityLocks(locks, nil, primary)
		}
		if err := verifyOwnedFile(lock, 0600); err != nil {
			return nil, cleanupLegacyAzureIdentityLocks(locks, lock, err)
		}
		if err := flockManagedFile(lock, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			primary := managedError(ManagedCacheUnavailable, "acquire legacy Azure Identity cache lock", err)
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				primary = managedError(ManagedCacheConflict, "acquire legacy Azure Identity cache lock", err)
			}
			return nil, cleanupLegacyAzureIdentityLocks(locks, lock, primary)
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func cleanupLegacyAzureIdentityLocks(locks []*os.File, current *os.File, primary error) error {
	errorsToJoin := []error{primary}
	if current != nil {
		if err := current.Close(); err != nil {
			errorsToJoin = append(errorsToJoin, managedError(ManagedCacheUnavailable, "close failed legacy Azure Identity cache lock", err))
		}
	}
	if err := releaseLegacyAzureIdentityLocks(locks, false); err != nil {
		errorsToJoin = append(errorsToJoin, err)
	}
	return errors.Join(errorsToJoin...)
}

func releaseLegacyAzureIdentityLocks(locks []*os.File, remove bool) error {
	var firstErr error
	for _, lock := range locks {
		if lock == nil {
			continue
		}
		path := lock.Name()
		if remove {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache lock", err)
			}
		}
		if err := flockManagedFile(lock, syscall.LOCK_UN); err != nil && firstErr == nil {
			firstErr = managedError(ManagedCacheUnavailable, "release legacy Azure Identity cache lock", err)
		}
		if err := lock.Close(); err != nil && firstErr == nil {
			firstErr = managedError(ManagedCacheUnavailable, "close legacy Azure Identity cache lock", err)
		}
		if remove {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = managedError(ManagedCacheConflict, "verify legacy Azure Identity cache lock deletion", nil)
			}
		}
	}
	return firstErr
}

func verifyOptionalOwnedPath(path string, directory bool, mode os.FileMode, required bool) error {
	info, err := os.Lstat(path) // #nosec G703 -- caller supplies an absolute path under an ownership-checked legacy cache root; this function rejects symlinks, wrong ownership, type, and mode
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
		// Search is recursive. Remove from the persistent child first so a
		// search through the user ring cannot repeatedly find a key that isn't
		// directly linked there.
		rings = []int{persistentRing, userRing}
	}
	for _, name := range legacyAzureIdentityCacheNames {
		for _, ring := range rings {
			for {
				keyID, searchErr := unix.KeyctlSearch(ring, "user", name, 0)
				if searchErr != nil {
					if isLegacyKeyAbsent(searchErr) {
						break
					}
					return managedError(ManagedCacheUnavailable, "locate legacy Azure Identity cache key", searchErr)
				}
				if _, unlinkErr := unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, ring, 0, 0); unlinkErr != nil {
					if isLegacyKeyAbsent(unlinkErr) {
						break
					}
					return managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache key", unlinkErr)
				}
			}
			if _, searchErr := unix.KeyctlSearch(ring, "user", name, 0); !isLegacyKeyAbsent(searchErr) {
				return managedError(ManagedCacheConflict, "verify legacy Azure Identity cache key deletion", nil)
			}
		}
	}
	return nil
}

func isLegacyKeyAbsent(err error) bool {
	return errors.Is(err, unix.ENOKEY) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EKEYEXPIRED) || errors.Is(err, unix.EKEYREVOKED)
}
