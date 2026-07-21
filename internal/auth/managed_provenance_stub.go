//go:build !linux || !cgo

package auth

import (
	"context"

	"github.com/nz365guy/cb365/internal/config"
)

func RecordManagedChannelMessageProvenance(context.Context, *config.Profile, ManagedChannelMessageTarget) (string, error) {
	return "", managedUnavailableBuildError()
}

func VerifyManagedChannelMessageProvenance(context.Context, *config.Profile, string, ManagedChannelMessageTarget) error {
	return managedUnavailableBuildError()
}

func DeleteManagedChannelMessageProvenance(context.Context, *config.Profile, string, ManagedChannelMessageTarget) error {
	return managedUnavailableBuildError()
}
