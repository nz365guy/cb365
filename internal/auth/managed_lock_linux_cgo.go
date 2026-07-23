//go:build linux && cgo

package auth

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type managedLockMetadata struct {
	Profile      string `json:"profile"`
	Host         string `json:"host"`
	PID          int    `json:"pid"`
	ProcessStart string `json:"processStart"`
}

type managedProfileLock struct {
	file *os.File
}

func acquireManagedProfileLock(profile, host string) (*managedProfileLock, error) {
	runtimeDir, err := managedRuntimeDir()
	if err != nil {
		return nil, err
	}
	lockDir := filepath.Join(runtimeDir, "cb365")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, managedError(ManagedCacheUnavailable, "create managed cache lock directory", err)
	}
	if err := verifyOwnedPath(lockDir, true, 0700); err != nil {
		return nil, err
	}

	digest := sha256.Sum256([]byte(profile))
	path := filepath.Join(lockDir, "managed-"+hex.EncodeToString(digest[:16])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_CLOEXEC, 0600) // #nosec G304 -- path is derived from an owned runtime directory and a digest
	if err != nil {
		return nil, managedError(ManagedCacheUnavailable, "open managed cache lock", err)
	}
	closeOnError := func(lockErr error) (*managedProfileLock, error) {
		_ = file.Close()
		return nil, lockErr
	}
	if err := verifyOwnedFile(file, 0600); err != nil {
		return closeOnError(err)
	}
	if err := flockManagedFile(file, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return closeOnError(managedError(ManagedCacheConflict, "acquire managed cache lock", err))
		}
		return closeOnError(managedError(ManagedCacheUnavailable, "acquire managed cache lock", err))
	}

	start, err := linuxProcessStart(os.Getpid())
	if err != nil {
		_ = flockManagedFile(file, syscall.LOCK_UN)
		return closeOnError(managedError(ManagedCacheUnavailable, "read managed cache lock owner", err))
	}
	if err := refuseLiveUnlockedOwner(file, host, start); err != nil {
		_ = flockManagedFile(file, syscall.LOCK_UN)
		return closeOnError(err)
	}
	metadata := managedLockMetadata{Profile: profile, Host: host, PID: os.Getpid(), ProcessStart: start}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		_ = flockManagedFile(file, syscall.LOCK_UN)
		return closeOnError(managedError(ManagedCacheUnavailable, "encode managed cache lock owner", err))
	}
	if err := file.Truncate(0); err == nil {
		_, err = file.Seek(0, 0)
	}
	if err == nil {
		_, err = file.Write(encoded)
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = flockManagedFile(file, syscall.LOCK_UN)
		return closeOnError(managedError(ManagedCacheUnavailable, "record managed cache lock owner", err))
	}
	return &managedProfileLock{file: file}, nil
}

func (l *managedProfileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := flockManagedFile(l.file, syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return managedError(ManagedCacheUnavailable, "release managed cache lock", unlockErr)
	}
	if closeErr != nil {
		return managedError(ManagedCacheUnavailable, "close managed cache lock", closeErr)
	}
	return nil
}

func flockManagedFile(file *os.File, operation int) error {
	fd := file.Fd()
	maxInt := uintptr(^uint(0) >> 1)
	if fd > maxInt {
		return managedError(ManagedCacheUnavailable, "validate managed cache lock descriptor", nil)
	}
	return syscall.Flock(int(fd), operation) // #nosec G115 -- descriptor range is checked before conversion
}

func managedRuntimeDir() (string, error) {
	uid := os.Geteuid()
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = filepath.Join("/run/user", strconv.Itoa(uid))
	}
	if !filepath.IsAbs(dir) {
		return "", managedError(ManagedCacheInvalid, "validate managed cache runtime directory", nil)
	}
	if err := verifyOwnedPath(dir, true, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func refuseLiveUnlockedOwner(file *os.File, host, currentStart string) error {
	if _, err := file.Seek(0, 0); err != nil {
		return managedError(ManagedCacheUnavailable, "read managed cache lock owner", err)
	}
	var existing managedLockMetadata
	if err := json.NewDecoder(file).Decode(&existing); err != nil {
		return nil
	}
	if existing.Host != host || existing.PID <= 0 || existing.ProcessStart == "" {
		return nil
	}
	start, err := linuxProcessStart(existing.PID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return managedError(ManagedCacheUnavailable, "verify stale managed cache lock", err)
	}
	if existing.PID == os.Getpid() && start == currentStart {
		return nil
	}
	if start == existing.ProcessStart {
		return managedError(ManagedCacheConflict, "refuse live stale managed cache lock", nil)
	}
	return nil
}

func linuxProcessStart(pid int) (string, error) {
	file, err := os.Open(filepath.Join("/proc", strconv.Itoa(pid), "stat")) // #nosec G304 -- pid is an integer
	if err != nil {
		return "", err
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) && line == "" {
		return "", err
	}
	end := strings.LastIndex(line, ")")
	if end < 0 {
		return "", fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(line[end+1:])
	// Field 22 (starttime) is index 19 after removing pid and comm.
	if len(fields) <= 19 {
		return "", fmt.Errorf("incomplete process stat")
	}
	return fields[19], nil
}

func verifyOwnedPath(path string, directory bool, requiredMode os.FileMode) error {
	info, err := os.Lstat(path) // #nosec G703 -- caller supplies a path derived from the fixed managed-cache root; this function rejects symlinks, wrong ownership, type, and mode
	if err != nil {
		return managedError(ManagedCacheUnavailable, "inspect managed cache path", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || info.Mode().Perm() != requiredMode {
		return managedError(ManagedCacheInvalid, "validate managed cache path permissions", nil)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return managedError(ManagedCacheInvalid, "validate managed cache path ownership", nil)
	}
	return nil
}

func verifyOwnedFile(file *os.File, requiredMode os.FileMode) error {
	info, err := file.Stat()
	if err != nil {
		return managedError(ManagedCacheUnavailable, "inspect managed cache file", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != requiredMode {
		return managedError(ManagedCacheInvalid, "validate managed cache file permissions", nil)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return managedError(ManagedCacheInvalid, "validate managed cache file ownership", nil)
	}
	return nil
}

var _ = time.Second
