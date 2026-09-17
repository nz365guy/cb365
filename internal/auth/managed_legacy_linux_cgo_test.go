//go:build linux && cgo

package auth

import "testing"

func TestIsAmbiguousLegacyKeyringFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ManagedCacheUnavailable is ambiguous (inconclusive keyring result)",
			err:  managedError(ManagedCacheUnavailable, "locate legacy Azure Identity cache key", nil),
			want: true,
		},
		{
			name: "ManagedCacheConflict is a confirmed leftover key, not ambiguous",
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

func TestCleanupLegacyDelegatedRejectsEmptyProfileNameRegardlessOfMode(t *testing.T) {
	if err := cleanupLegacyDelegated("", true); err == nil {
		t.Fatal("expected an error for an empty profile name in best-effort mode")
	}
	if err := cleanupLegacyDelegated("", false); err == nil {
		t.Fatal("expected an error for an empty profile name in strict mode")
	}
}
