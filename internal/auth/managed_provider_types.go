package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/nz365guy/cb365/internal/config"
)

const (
	managedMigrationComplete = "complete"
	managedMigrationCleanup  = "cleanup_required"
)

// ManagedLoginOptions contains non-secret Bitwarden references. The machine
// access token is accepted only from the BWS_ACCESS_TOKEN runtime injection
// boundary and therefore cannot be supplied through this API or a CLI flag.
type ManagedLoginOptions struct {
	OrganisationID string
	ProjectID      string
}

// ManagedLoginResult contains the token only for immediate in-process use.
// Callers must persist Metadata, never Token.
type ManagedLoginResult struct {
	Token    azcore.AccessToken
	Username string
	Metadata config.ManagedDelegatedMetadata
}

// ManagedDeviceCodePrompt renders the non-secret operator message returned by
// Entra during an explicitly requested interactive login.
type ManagedDeviceCodePrompt func(context.Context, azidentity.DeviceCodeMessage) error

func managedProfileBinding(profile *config.Profile) (managedBinding, error) {
	if profile == nil || profile.ManagedDelegated == nil {
		return managedBinding{}, managedError(ManagedCacheInvalid, "load managed profile metadata", nil)
	}
	metadata := profile.ManagedDelegated
	if metadata.MigrationState != managedMigrationComplete {
		return managedBinding{}, managedError(ManagedCacheUnavailable, "complete delegated credential migration", nil)
	}
	if metadata.HomeAccountID == "" || metadata.SecretID == "" || metadata.OrganisationID == "" ||
		metadata.ProjectID == "" || metadata.AssignedHost == "" {
		return managedBinding{}, managedError(ManagedCacheInvalid, "validate managed profile metadata", nil)
	}
	return managedBinding{
		TenantID:      profile.TenantID,
		ClientID:      profile.ClientID,
		HomeAccountID: metadata.HomeAccountID,
		Profile:       profile.Name,
	}, nil
}

func mapManagedAcquisitionError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := ManagedErrorClassOf(err); ok {
		return err
	}
	return managedError(ReauthenticationRequired, operation, err)
}

func managedUnavailableBuildError() error {
	return fmt.Errorf("managed delegated authentication unavailable on this build")
}
