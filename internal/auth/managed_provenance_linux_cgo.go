//go:build linux && cgo

package auth

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/nz365guy/cb365/internal/config"
)

type managedProvenanceStore interface {
	Get(context.Context, string) (*managedSecret, error)
	CreateProvenance(context.Context, string, string, string, string) (*managedSecret, error)
	Delete(context.Context, string) error
}

func (s *sdkSecretStore) CreateProvenance(ctx context.Context, key, value, organisationID, projectID string) (*managedSecret, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	secret, err := s.secrets.Create(key, value, "cb365 Teams own-message provenance", organisationID, []string{projectID})
	if err != nil {
		return nil, errors.New("Bitwarden provenance create failed")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return managedSecretFromSDK(secret), nil
}

// RecordManagedChannelMessageProvenance creates and verifies the BWS record
// that proves a managed delegated profile sent one root channel message.
func RecordManagedChannelMessageProvenance(ctx context.Context, profile *config.Profile, target ManagedChannelMessageTarget) (string, error) {
	host, binding, err := validateManagedProvenanceHost(profile, target)
	if err != nil {
		return "", err
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return "", err
	}
	defer closeStore()
	return recordManagedChannelMessageProvenance(ctx, store, profile, binding, target, host, time.Now)
}

// VerifyManagedChannelMessageProvenance fails closed unless the immutable BWS
// record matches the exact profile, tenant, client, account, host, and target.
func VerifyManagedChannelMessageProvenance(ctx context.Context, profile *config.Profile, secretID string, target ManagedChannelMessageTarget) error {
	host, binding, err := validateManagedProvenanceHost(profile, target)
	if err != nil {
		return err
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return err
	}
	defer lock.Close()
	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return err
	}
	defer closeStore()
	return verifyManagedChannelMessageProvenance(ctx, store, profile, binding, secretID, target, host)
}

// DeleteManagedChannelMessageProvenance verifies, deletes, and proves absence
// of a consumed provenance record. It never deletes an unverified record.
func DeleteManagedChannelMessageProvenance(ctx context.Context, profile *config.Profile, secretID string, target ManagedChannelMessageTarget) error {
	host, binding, err := validateManagedProvenanceHost(profile, target)
	if err != nil {
		return err
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return err
	}
	defer lock.Close()
	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return err
	}
	defer closeStore()
	if err := verifyManagedChannelMessageProvenance(ctx, store, profile, binding, secretID, target, host); err != nil {
		return err
	}
	if err := store.Delete(ctx, secretID); err != nil {
		return managedError(ManagedCacheUnavailable, "delete channel-message provenance", err)
	}
	_, err = store.Get(ctx, secretID)
	if !errors.Is(err, errManagedSecretNotFound) {
		return managedError(ManagedCacheConflict, "verify channel-message provenance deletion", nil)
	}
	return nil
}

func validateManagedProvenanceHost(profile *config.Profile, target ManagedChannelMessageTarget) (string, managedBinding, error) {
	if !target.valid() {
		return "", managedBinding{}, managedError(ManagedCacheInvalid, "validate channel-message target", nil)
	}
	binding, err := managedProfileBinding(profile)
	if err != nil {
		return "", managedBinding{}, err
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "", managedBinding{}, managedError(ManagedCacheUnavailable, "resolve assigned host", err)
	}
	if profile.ManagedDelegated.AssignedHost != host {
		return "", managedBinding{}, managedError(ManagedCacheConflict, "use channel-message provenance on assigned host", nil)
	}
	return host, binding, nil
}

func recordManagedChannelMessageProvenance(
	ctx context.Context,
	store managedProvenanceStore,
	profile *config.Profile,
	binding managedBinding,
	target ManagedChannelMessageTarget,
	host string,
	now func() time.Time,
) (string, error) {
	if store == nil || profile == nil || profile.ManagedDelegated == nil || !binding.valid() || !target.valid() || host == "" {
		return "", managedError(ManagedCacheInvalid, "record channel-message provenance", nil)
	}
	envelope := managedChannelMessageProvenance{
		SchemaVersion: managedChannelMessageProvenanceSchema,
		Binding:       binding,
		Target:        target,
		CreatedAt:     now().UTC().Format(time.RFC3339Nano),
		Writer:        host,
	}
	encoded, err := marshalManagedChannelMessageProvenance(envelope)
	if err != nil {
		return "", managedError(ManagedCacheInvalid, "encode channel-message provenance", err)
	}
	secret, err := store.CreateProvenance(
		ctx,
		managedChannelMessageSecretKey(binding, target),
		string(encoded),
		profile.ManagedDelegated.OrganisationID,
		profile.ManagedDelegated.ProjectID,
	)
	if err != nil || secret == nil || secret.ID == "" {
		return "", managedError(ManagedCacheUnavailable, "write channel-message provenance", err)
	}
	if err := verifyManagedChannelMessageProvenance(ctx, store, profile, binding, secret.ID, target, host); err != nil {
		_ = store.Delete(ctx, secret.ID)
		return "", err
	}
	return secret.ID, nil
}

func verifyManagedChannelMessageProvenance(
	ctx context.Context,
	store managedProvenanceStore,
	profile *config.Profile,
	binding managedBinding,
	secretID string,
	target ManagedChannelMessageTarget,
	host string,
) error {
	if store == nil || profile == nil || profile.ManagedDelegated == nil || secretID == "" || !binding.valid() || !target.valid() || host == "" {
		return managedError(ManagedCacheInvalid, "verify channel-message provenance", nil)
	}
	secret, err := store.Get(ctx, secretID)
	if err != nil {
		return managedError(ManagedCacheUnavailable, "read channel-message provenance", err)
	}
	if secret == nil || secret.ID != secretID ||
		secret.OrganisationID != profile.ManagedDelegated.OrganisationID ||
		secret.ProjectID != profile.ManagedDelegated.ProjectID ||
		secret.Key != managedChannelMessageSecretKey(binding, target) {
		return managedError(ManagedCacheInvalid, "validate channel-message provenance location", nil)
	}
	envelope, err := unmarshalManagedChannelMessageProvenance([]byte(secret.Value))
	if err != nil || envelope.SchemaVersion != managedChannelMessageProvenanceSchema ||
		envelope.Binding != binding || envelope.Target != target || envelope.Writer != host {
		return managedError(ManagedCacheInvalid, "validate channel-message provenance binding", err)
	}
	return nil
}
