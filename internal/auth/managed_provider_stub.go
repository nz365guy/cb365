//go:build !linux || !cgo

package auth

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/nz365guy/cb365/internal/config"
)

// ManagedDelegatedAvailable reports whether this binary contains the pinned
// Linux/cgo Bitwarden SDK provider.
func ManagedDelegatedAvailable() bool { return false }

func LoginManagedDelegated(context.Context, *config.Profile, ManagedLoginOptions, bool, ManagedDeviceCodePrompt) (*ManagedLoginResult, error) {
	return nil, managedUnavailableBuildError()
}

func MigrateManagedDelegated(context.Context, *config.Profile, ManagedLoginOptions, bool, ManagedDeviceCodePrompt) (*ManagedLoginResult, error) {
	return nil, managedUnavailableBuildError()
}

func ResumeManagedDelegatedMigration(context.Context, *config.Profile) error {
	return managedUnavailableBuildError()
}

func NewManagedDelegatedCredential(*config.Profile, bool) (azcore.TokenCredential, error) {
	return nil, managedUnavailableBuildError()
}

func DeleteManagedDelegated(context.Context, *config.Profile) error {
	return managedUnavailableBuildError()
}

func DiscardManagedDelegatedMigration(context.Context, *config.Profile) error {
	return managedUnavailableBuildError()
}

func DiscardManagedDelegatedRecord(context.Context, *config.Profile) error {
	return managedUnavailableBuildError()
}
