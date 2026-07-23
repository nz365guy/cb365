//go:build linux && cgo

package auth

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	sdk "github.com/bitwarden/sdk-go/v2"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/graph"
)

const (
	bitwardenEUAPIURL       = "https://api.bitwarden.eu"
	bitwardenEUIdentityURL  = "https://identity.bitwarden.eu"
	bitwardenAccessTokenEnv = "BWS_ACCESS_TOKEN"
)

// ManagedDelegatedAvailable reports whether this binary contains the pinned
// Linux/cgo Bitwarden SDK provider.
func ManagedDelegatedAvailable() bool { return true }

func LoginManagedDelegated(
	ctx context.Context,
	profile *config.Profile,
	options ManagedLoginOptions,
	ipv4Only bool,
	prompt ManagedDeviceCodePrompt,
) (*ManagedLoginResult, error) {
	if err := validateManagedLoginInput(profile, options); err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return nil, managedError(ManagedCacheUnavailable, "resolve assigned host", err)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	return loginManagedLocked(ctx, profile, options, ipv4Only, prompt, host, "")
}

func MigrateManagedDelegated(
	ctx context.Context,
	profile *config.Profile,
	options ManagedLoginOptions,
	ipv4Only bool,
	prompt ManagedDeviceCodePrompt,
) (*ManagedLoginResult, error) {
	if err := validateManagedLoginInput(profile, options); err != nil {
		return nil, err
	}
	if profile.ManagedDelegated != nil {
		return nil, managedError(ManagedCacheConflict, "start delegated credential migration", nil)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return nil, managedError(ManagedCacheUnavailable, "resolve assigned host", err)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	legacy, err := inspectLegacyDelegated(profile)
	if err != nil {
		return nil, err
	}
	result, err := loginManagedLocked(ctx, profile, options, ipv4Only, prompt, host, legacy.homeAccountID)
	if err != nil {
		return nil, err
	}
	// Persist cleanup_required in the non-secret profile before deleting any
	// source layer. This makes interruption and config-save failures resumable
	// while workload commands remain fail closed.
	result.Metadata.MigrationState = managedMigrationCleanup
	return result, nil
}

func ResumeManagedDelegatedMigration(_ context.Context, profile *config.Profile) error {
	if profile == nil || profile.ManagedDelegated == nil || profile.ManagedDelegated.MigrationState != managedMigrationCleanup {
		return managedError(ManagedCacheInvalid, "resume delegated credential migration", nil)
	}
	host, err := os.Hostname()
	if err != nil || host == "" || profile.ManagedDelegated.AssignedHost != host {
		return managedError(ManagedCacheConflict, "resume delegated credential migration on assigned host", err)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return err
	}
	defer lock.Close()
	return cleanupLegacyDelegated(profile.Name)
}

func NewManagedDelegatedCredential(profile *config.Profile, ipv4Only bool) (azcore.TokenCredential, error) {
	if _, err := managedProfileBinding(profile); err != nil {
		return nil, err
	}
	return &managedDelegatedCredential{profile: cloneProfile(profile), ipv4Only: ipv4Only}, nil
}

type managedDelegatedCredential struct {
	profile  *config.Profile
	ipv4Only bool
}

func (c *managedDelegatedCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	profile := c.profile
	binding, err := managedProfileBinding(profile)
	if err != nil {
		return azcore.AccessToken{}, err
	}
	host, err := os.Hostname()
	if err != nil || host == "" || profile.ManagedDelegated.AssignedHost != host {
		return azcore.AccessToken{}, managedError(ManagedCacheConflict, "acquire token on assigned host", err)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return azcore.AccessToken{}, err
	}
	defer lock.Close()

	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return azcore.AccessToken{}, err
	}
	defer closeStore()
	metadata := profile.ManagedDelegated
	adapter, err := newManagedCacheAdapter(
		store,
		binding,
		metadata.OrganisationID,
		metadata.ProjectID,
		metadata.SecretID,
		host,
		false,
	)
	if err != nil {
		return azcore.AccessToken{}, err
	}
	client, err := newManagedPublicClient(profile, adapter, c.ipv4Only)
	if err != nil {
		return azcore.AccessToken{}, err
	}
	scopes := options.Scopes
	if len(scopes) == 0 {
		scopes = GraphScopes(profile.Scopes)
	}
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}
	result, err := client.AcquireTokenSilent(
		ctx,
		scopes,
		public.WithSilentAccount(public.Account{
			HomeAccountID:     binding.HomeAccountID,
			PreferredUsername: profile.Username,
		}),
		public.WithTenantID(profile.TenantID),
	)
	if err != nil {
		return azcore.AccessToken{}, mapManagedAcquisitionError(err, "acquire managed delegated token")
	}
	if result.Account.HomeAccountID != "" && result.Account.HomeAccountID != binding.HomeAccountID {
		return azcore.AccessToken{}, managedError(ManagedCacheInvalid, "validate acquired account binding", nil)
	}
	return azcore.AccessToken{Token: result.AccessToken, ExpiresOn: result.ExpiresOn}, nil
}

func DeleteManagedDelegated(ctx context.Context, profile *config.Profile) error {
	if profile == nil || profile.AuthMode != config.AuthModeDelegated {
		return managedError(ManagedCacheInvalid, "delete managed delegated credential", nil)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return managedError(ManagedCacheUnavailable, "resolve assigned host", err)
	}
	if profile.ManagedDelegated != nil && profile.ManagedDelegated.AssignedHost != host {
		return managedError(ManagedCacheConflict, "delete managed cache on assigned host", nil)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return err
	}
	defer lock.Close()

	if err := deleteManagedChannelMessageProvenanceLocked(ctx, profile, host); err != nil {
		return err
	}
	if err := deleteManagedRecordLocked(ctx, profile, host); err != nil {
		return err
	}
	return cleanupLegacyDelegated(profile.Name)
}

// DiscardManagedDelegatedMigration removes only the newly-created BWS record.
// It is used when non-secret migration metadata cannot be persisted; the
// verified legacy source remains untouched and can be retried safely.
func DiscardManagedDelegatedMigration(ctx context.Context, profile *config.Profile) error {
	if profile == nil || profile.AuthMode != config.AuthModeDelegated || profile.ManagedDelegated == nil ||
		profile.ManagedDelegated.MigrationState != managedMigrationCleanup {
		return managedError(ManagedCacheInvalid, "discard delegated credential migration", nil)
	}
	return DiscardManagedDelegatedRecord(ctx, profile)
}

// DiscardManagedDelegatedRecord removes a newly-created BWS record without
// touching any legacy cache. It is only a transaction rollback helper.
func DiscardManagedDelegatedRecord(ctx context.Context, profile *config.Profile) error {
	if profile == nil || profile.AuthMode != config.AuthModeDelegated || profile.ManagedDelegated == nil {
		return managedError(ManagedCacheInvalid, "discard managed delegated record", nil)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return managedError(ManagedCacheUnavailable, "resolve assigned host", err)
	}
	if profile.ManagedDelegated.AssignedHost != host {
		return managedError(ManagedCacheConflict, "discard delegated credential migration on assigned host", nil)
	}
	lock, err := acquireManagedProfileLock(profile.Name, host)
	if err != nil {
		return err
	}
	defer lock.Close()
	return deleteManagedRecordLocked(ctx, profile, host)
}

func deleteManagedRecordLocked(ctx context.Context, profile *config.Profile, host string) error {
	if profile.ManagedDelegated == nil || profile.ManagedDelegated.SecretID == "" {
		return nil
	}
	binding := managedBinding{
		TenantID:      profile.TenantID,
		ClientID:      profile.ClientID,
		HomeAccountID: profile.ManagedDelegated.HomeAccountID,
		Profile:       profile.Name,
	}
	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return err
	}
	defer closeStore()
	adapter, err := newManagedCacheAdapter(
		store,
		binding,
		profile.ManagedDelegated.OrganisationID,
		profile.ManagedDelegated.ProjectID,
		profile.ManagedDelegated.SecretID,
		host,
		false,
	)
	if err != nil {
		return err
	}
	return adapter.deleteAndVerify(ctx)
}

func loginManagedLocked(
	ctx context.Context,
	profile *config.Profile,
	options ManagedLoginOptions,
	ipv4Only bool,
	prompt ManagedDeviceCodePrompt,
	host string,
	expectedHomeAccountID string,
) (*ManagedLoginResult, error) {
	store, closeStore, err := openSDKSecretStore()
	if err != nil {
		return nil, err
	}
	defer closeStore()
	adapter, err := newManagedCacheAdapter(
		store,
		managedBinding{TenantID: profile.TenantID, ClientID: profile.ClientID, Profile: profile.Name},
		options.OrganisationID,
		options.ProjectID,
		"",
		host,
		true,
	)
	if err != nil {
		return nil, err
	}
	client, err := newManagedPublicClient(profile, adapter, ipv4Only)
	if err != nil {
		return nil, err
	}
	scopes := GraphScopes(profile.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}
	deviceCode, err := client.AcquireTokenByDeviceCode(ctx, scopes, public.WithTenantID(profile.TenantID))
	if err != nil {
		return nil, mapManagedAcquisitionError(err, "start managed delegated login")
	}
	if prompt != nil {
		message := azidentity.DeviceCodeMessage{
			UserCode:        deviceCode.Result.UserCode,
			VerificationURL: deviceCode.Result.VerificationURL,
			Message:         deviceCode.Result.Message,
		}
		if err := prompt(ctx, message); err != nil {
			return nil, managedError(ReauthenticationRequired, "present managed delegated login", err)
		}
	}
	result, err := deviceCode.AuthenticationResult(ctx)
	if err != nil {
		return nil, mapManagedAcquisitionError(err, "complete managed delegated login")
	}
	homeAccountID := result.Account.HomeAccountID
	if homeAccountID == "" || (expectedHomeAccountID != "" && expectedHomeAccountID != homeAccountID) {
		return nil, managedError(ManagedCacheInvalid, "validate managed login account binding", nil)
	}
	if err := adapter.commitInitial(ctx, homeAccountID); err != nil {
		return nil, err
	}
	return &ManagedLoginResult{
		Token:    azcore.AccessToken{Token: result.AccessToken, ExpiresOn: result.ExpiresOn},
		Username: result.Account.PreferredUsername,
		Metadata: config.ManagedDelegatedMetadata{
			HomeAccountID:  homeAccountID,
			SecretID:       adapter.secretID,
			OrganisationID: options.OrganisationID,
			ProjectID:      options.ProjectID,
			AssignedHost:   host,
			MigrationState: managedMigrationComplete,
		},
	}, nil
}

func validateManagedLoginInput(profile *config.Profile, options ManagedLoginOptions) error {
	if profile == nil || profile.Name == "" || profile.TenantID == "" || profile.ClientID == "" ||
		profile.AuthMode != config.AuthModeDelegated || options.OrganisationID == "" || options.ProjectID == "" {
		return managedError(ManagedCacheInvalid, "validate managed delegated login", nil)
	}
	if os.Getenv("AZURE_SDK_GO_LOGGING") != "" {
		return managedError(ManagedCacheInvalid, "enforce managed delegated logging boundary", nil)
	}
	return nil
}

func newManagedPublicClient(profile *config.Profile, adapter *managedCacheAdapter, ipv4Only bool) (public.Client, error) {
	authority := "https://login.microsoftonline.com/" + url.PathEscape(profile.TenantID)
	options := []public.Option{
		public.WithAuthority(authority),
		public.WithCache(adapter),
		public.WithClientCapabilities([]string{"CP1"}),
	}
	if ipv4Only {
		options = append(options, public.WithHTTPClient(graph.NewIPv4HTTPClient()))
	}
	client, err := public.New(profile.ClientID, options...)
	if err != nil {
		return public.Client{}, managedError(ManagedCacheInvalid, "create managed delegated client", err)
	}
	return client, nil
}

func cloneProfile(profile *config.Profile) *config.Profile {
	cloned := *profile
	cloned.Scopes = append([]string(nil), profile.Scopes...)
	if profile.ManagedDelegated != nil {
		metadata := *profile.ManagedDelegated
		if profile.ManagedDelegated.ChannelMessageProvenance != nil {
			metadata.ChannelMessageProvenance = make(map[string]string, len(profile.ManagedDelegated.ChannelMessageProvenance))
			for key, value := range profile.ManagedDelegated.ChannelMessageProvenance {
				metadata.ChannelMessageProvenance[key] = value
			}
		}
		cloned.ManagedDelegated = &metadata
	}
	return &cloned
}

type sdkSecretStore struct {
	client  sdk.BitwardenClientInterface
	secrets sdk.SecretsInterface
}

func openSDKSecretStore() (*sdkSecretStore, func(), error) {
	accessToken := os.Getenv(bitwardenAccessTokenEnv)
	if accessToken == "" {
		return nil, func() {}, managedError(ManagedCacheUnavailable, "load Bitwarden runtime credential", nil)
	}
	apiURL := bitwardenEUAPIURL
	identityURL := bitwardenEUIdentityURL
	client, err := sdk.NewBitwardenClient(&apiURL, &identityURL)
	if err != nil {
		return nil, func() {}, managedError(ManagedCacheUnavailable, "initialise Bitwarden EU client", err)
	}
	closeClient := func() { client.Close() }
	if err := client.AccessTokenLogin(accessToken, nil); err != nil {
		closeClient()
		return nil, func() {}, managedError(ManagedCacheUnavailable, "authenticate Bitwarden machine account", err)
	}
	return &sdkSecretStore{client: client, secrets: client.Secrets()}, closeClient, nil
}

func (s *sdkSecretStore) Get(ctx context.Context, id string) (*managedSecret, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	secret, err := s.secrets.Get(id)
	if err != nil {
		if sdkNotFound(err) {
			return nil, errManagedSecretNotFound
		}
		return nil, errors.New("Bitwarden secret read failed")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return managedSecretFromSDK(secret), nil
}

func (s *sdkSecretStore) Create(ctx context.Context, key, value, organisationID, projectID string) (*managedSecret, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	secret, err := s.secrets.Create(key, value, "cb365 managed delegated cache", organisationID, []string{projectID})
	if err != nil {
		return nil, errors.New("Bitwarden secret create failed")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return managedSecretFromSDK(secret), nil
}

func (s *sdkSecretStore) Update(ctx context.Context, id, key, value, organisationID, projectID string) (*managedSecret, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	secret, err := s.secrets.Update(id, key, value, "cb365 managed delegated cache", organisationID, []string{projectID})
	if err != nil {
		return nil, errors.New("Bitwarden secret update failed")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return managedSecretFromSDK(secret), nil
}

func (s *sdkSecretStore) Delete(ctx context.Context, id string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	response, err := s.secrets.Delete([]string{id})
	if err != nil {
		if sdkNotFound(err) {
			return nil
		}
		return errors.New("Bitwarden secret delete failed")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if response == nil || len(response.Data) != 1 || response.Data[0].ID != id || response.Data[0].Error != nil {
		return errors.New("Bitwarden secret deletion was not acknowledged")
	}
	return nil
}

func managedSecretFromSDK(secret *sdk.SecretResponse) *managedSecret {
	if secret == nil {
		return nil
	}
	projectID := ""
	if secret.ProjectID != nil {
		projectID = *secret.ProjectID
	}
	return &managedSecret{
		ID:             secret.ID,
		Key:            secret.Key,
		Value:          secret.Value,
		OrganisationID: secret.OrganizationID,
		ProjectID:      projectID,
	}
}

func sdkNotFound(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "404")
}

func isManagedSecretAbsent(err error) bool {
	return errors.Is(err, errManagedSecretNotFound)
}
