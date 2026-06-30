//go:build !darwin

package auth

import (
	azcache "github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
)

// init sets msalCache to a persistent MSAL token cache on platforms whose
// cache backend builds without cgo (Linux and Windows). On macOS the backend
// requires cgo; see credential_cache_darwin.go.
func init() {
	c, err := azcache.New(&azcache.Options{Name: "cb365"})
	if err != nil {
		// Non-fatal: msalCache stays zero-value and MSAL falls back to
		// in-memory only, so the user just will not get persistent refresh tokens.
		return
	}
	msalCache = c
}
