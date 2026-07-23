package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
//  Auth error-propagation regression tests
// ──────────────────────────────────────────────
//
// These tests lock down the contract that auth and token-refresh failures
// must propagate as non-zero exit codes, not silent stderr-only warnings.
//
// Background: a refreshed-token persistence failure (auth.StoreToken)
// is recoverable in the sense that the next call will re-refresh, but
// discarding the error with `_ = auth.StoreToken(...)` means a real
// failure (encrypted-store corruption, disk full, keyring contention)
// can go completely unnoticed by scripts parsing exit codes.
//
// Cobra propagates RunE errors as exit code 1 via main.go. Any new code
// path that calls auth.StoreToken inside a RunE or inside a function
// reachable from RunE must check the returned error.
//
// Shell pipe gotcha (worth noting for users):
//   cb365 onedrive ls --path "/" | head -5
//   echo $?    # this is head's exit, not cb365's.
//             # Use `set -o pipefail` or `${PIPESTATUS[0]}` to get cb365's
//             # real exit when piping.

// discardedStoreToken matches `_ = auth.StoreToken(...)` and similar variants.
// Any match means a refresh-store failure could be silently swallowed.
var discardedStoreToken = regexp.MustCompile(`_\s*[:=]?=?\s*auth\.StoreToken\b`)

// criticalAuthFiles are the files that contain token-refresh paths reachable
// from RunE handlers. New refresh code paths added to other files should
// also be exercised by this contract.
var criticalAuthFiles = []string{
	"auth.go",
	"todo.go",
}

func TestNoDiscardedStoreTokenInCriticalPaths(t *testing.T) {
	for _, name := range criticalAuthFiles {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			src := string(b)
			if discardedStoreToken.MatchString(src) {
				// Surface the offending line numbers to help the developer.
				lines := strings.Split(src, "\n")
				var hits []string
				for i, line := range lines {
					if discardedStoreToken.MatchString(line) {
						hits = append(hits, line)
						_ = i
					}
				}
				t.Errorf("%s contains a discarded auth.StoreToken call. "+
					"Token-persistence failures in refresh paths must propagate. "+
					"Offending line(s):\n  %s",
					name, strings.Join(hits, "\n  "))
			}
		})
	}
}

// TestAuthStatusRefreshPathsReturnError verifies, by source inspection, that
// every refresh branch inside `cb365 auth status` returns an error from
// StoreToken rather than discarding it. This is a structural check
// complementary to the regex test above, looking specifically inside the
// auth-status command body.
func TestAuthStatusRefreshPathsReturnError(t *testing.T) {
	b, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	src := string(b)

	// Locate the auth status RunE body. Defensive: if the marker moves,
	// the test should still flag the missing structure rather than pass
	// silently.
	start := strings.Index(src, "var authStatusCmd = &cobra.Command{")
	if start == -1 {
		t.Fatal("could not find authStatusCmd in auth.go")
	}
	// Take a generous slice that covers the whole status command.
	end := start + 4000
	if end > len(src) {
		end = len(src)
	}
	body := src[start:end]

	// Every StoreToken call inside the status command body must be inside
	// an `if err := ...; err != nil { return ... }` construct.
	storeCalls := strings.Count(body, "auth.StoreToken(profileName, cache)")
	if storeCalls == 0 {
		t.Fatal("auth status body contains no auth.StoreToken calls (refactor likely)")
	}

	// Count protected calls (those preceded by `if err := auth.StoreToken`).
	protected := strings.Count(body, "if err := auth.StoreToken(profileName, cache)")
	if protected < storeCalls {
		t.Errorf("auth status: %d StoreToken call(s), only %d are error-checked. "+
			"All refresh-path persistence failures must propagate.",
			storeCalls, protected)
	}
}
