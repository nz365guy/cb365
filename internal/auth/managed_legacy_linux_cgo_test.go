//go:build linux && cgo

package auth

import (
	"errors"
	"testing"
)

func TestIsAmbiguousLegacyKeyringFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "an inconclusive-marked ManagedCacheUnavailable is ambiguous",
			err:  inconclusiveLegacyKeyringError(ManagedCacheUnavailable, "locate legacy Azure Identity cache key", nil),
			want: true,
		},
		{
			name: "a plain (unmarked) ManagedCacheUnavailable is NOT ambiguous -- e.g. a key that was found but could not be deleted is a confirmed problem, not a search ambiguity, even though it shares the same class",
			err:  managedError(ManagedCacheUnavailable, "delete legacy Azure Identity cache key", nil),
			want: false,
		},
		{
			name: "ManagedCacheConflict (confirmed still present) is never ambiguous",
			err:  managedError(ManagedCacheConflict, "verify legacy Azure Identity cache key deletion", nil),
			want: false,
		},
		{
			name: "ManagedCacheInvalid is a real validation failure, not ambiguous",
			err:  managedError(ManagedCacheInvalid, "clean up legacy delegated credential", nil),
			want: false,
		},
		{
			name: "a non-ManagedError is never treated as ambiguous",
			err:  errNotFound,
			want: false,
		},
		{
			name: "nil is never ambiguous",
			err:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAmbiguousLegacyKeyringFailure(tc.err); got != tc.want {
				t.Fatalf("isAmbiguousLegacyKeyringFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestInconclusiveLegacyKeyringErrorStillClassifiesCorrectly(t *testing.T) {
	// The sentinel wrapper must not break ManagedErrorClassOf for every other
	// existing consumer of these errors -- it only adds a narrower, separate
	// way to identify the ambiguous subset.
	err := inconclusiveLegacyKeyringError(ManagedCacheUnavailable, "open legacy Azure Identity keyring", nil)
	class, ok := ManagedErrorClassOf(err)
	if !ok {
		t.Fatal("expected ManagedErrorClassOf to recognise the wrapped error")
	}
	if class != ManagedCacheUnavailable {
		t.Fatalf("class = %v, want %v", class, ManagedCacheUnavailable)
	}
	var managed *ManagedError
	if !errors.As(err, &managed) {
		t.Fatal("expected errors.As to unwrap to the underlying *ManagedError")
	}
}

func TestCleanupLegacyDelegatedRejectsEmptyProfileNameRegardlessOfMode(t *testing.T) {
	if err := cleanupLegacyDelegated("", true); err == nil {
		t.Fatal("expected an error for an empty profile name in best-effort mode")
	}
	if err := cleanupLegacyDelegated("", false); err == nil {
		t.Fatal("expected an error for an empty profile name in strict mode")
	}
}
