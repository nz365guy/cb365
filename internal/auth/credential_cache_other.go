//go:build !darwin

package auth

import (
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azcache "github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
)

var (
	legacyCacheOnce sync.Once
	legacyCache     azidentity.Cache
	legacyCacheErr  error
)

// loadLegacyMSALCache is intentionally lazy. Managed delegated commands must
// never initialise Azure Identity's local cache merely by importing auth.
func loadLegacyMSALCache() (azidentity.Cache, error) {
	legacyCacheOnce.Do(func() {
		legacyCache, legacyCacheErr = azcache.New(&azcache.Options{Name: "cb365"})
	})
	return legacyCache, legacyCacheErr
}
