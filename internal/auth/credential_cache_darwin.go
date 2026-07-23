//go:build darwin

package auth

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func loadLegacyMSALCache() (azidentity.Cache, error) {
	return azidentity.Cache{}, fmt.Errorf("legacy delegated persistent cache unavailable on this build")
}
