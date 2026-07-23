package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
)

const managedCacheSchema = "cb365.msal-cache/v2"

var (
	errManagedSecretNotFound = errors.New("managed secret not found")
	digestPattern            = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type managedBinding struct {
	TenantID      string `json:"tenantId"`
	ClientID      string `json:"clientId"`
	HomeAccountID string `json:"homeAccountId"`
	Profile       string `json:"profile"`
}

func (b managedBinding) valid() bool {
	return b.TenantID != "" && b.ClientID != "" && b.HomeAccountID != "" && b.Profile != ""
}

type managedEnvelope struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Binding        managedBinding  `json:"binding"`
	Generation     uint64          `json:"generation"`
	PreviousDigest *string         `json:"previousDigest"`
	Cache          json.RawMessage `json:"cache"`
	UpdatedAt      string          `json:"updatedAt"`
	Writer         string          `json:"writer"`
}

type managedSecret struct {
	ID             string
	Key            string
	Value          string
	OrganisationID string
	ProjectID      string
}

type managedSecretStore interface {
	Get(context.Context, string) (*managedSecret, error)
	Create(context.Context, string, string, string, string) (*managedSecret, error)
	Update(context.Context, string, string, string, string, string) (*managedSecret, error)
	Delete(context.Context, string) error
}

type managedCacheAdapter struct {
	store          managedSecretStore
	binding        managedBinding
	organisationID string
	projectID      string
	secretID       string
	host           string
	allowCreate    bool
	now            func() time.Time

	loadedGeneration uint64
	loadedDigest     string
	stagedCache      []byte
}

var _ msalcache.ExportReplace = (*managedCacheAdapter)(nil)

func newManagedCacheAdapter(
	store managedSecretStore,
	binding managedBinding,
	organisationID string,
	projectID string,
	secretID string,
	host string,
	allowCreate bool,
) (*managedCacheAdapter, error) {
	if store == nil || binding.TenantID == "" || binding.ClientID == "" || binding.Profile == "" ||
		organisationID == "" || projectID == "" || host == "" {
		return nil, managedError(ManagedCacheInvalid, "initialise managed cache", nil)
	}
	if !allowCreate && (binding.HomeAccountID == "" || secretID == "") {
		return nil, managedError(ManagedCacheInvalid, "initialise managed cache", nil)
	}
	return &managedCacheAdapter{
		store:          store,
		binding:        binding,
		organisationID: organisationID,
		projectID:      projectID,
		secretID:       secretID,
		host:           host,
		allowCreate:    allowCreate,
		now:            time.Now,
	}, nil
}

func (a *managedCacheAdapter) Replace(ctx context.Context, target msalcache.Unmarshaler, _ msalcache.ReplaceHints) error {
	ctx, cancel := managedOperationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.secretID == "" {
		if a.allowCreate {
			a.loadedGeneration = 0
			a.loadedDigest = ""
			return nil
		}
		return managedError(ManagedCacheUnavailable, "read managed cache", errManagedSecretNotFound)
	}

	secret, envelope, digest, err := a.readAndValidate(ctx, a.secretID)
	if err != nil {
		return err
	}
	if secret.ID != a.secretID {
		return managedError(ManagedCacheInvalid, "validate managed cache reference", nil)
	}
	if err := target.Unmarshal(bytes.Clone(envelope.Cache)); err != nil {
		return managedError(ManagedCacheInvalid, "replace managed cache", err)
	}
	a.loadedGeneration = envelope.Generation
	a.loadedDigest = digest
	return nil
}

func (a *managedCacheAdapter) Export(ctx context.Context, source msalcache.Marshaler, _ msalcache.ExportHints) error {
	ctx, cancel := managedOperationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := source.Marshal()
	if err != nil {
		return managedError(ManagedCacheInvalid, "export managed cache", err)
	}
	if !json.Valid(raw) {
		return managedError(ManagedCacheInvalid, "export managed cache", nil)
	}

	// A first device-code acquisition cannot expose the account binding until
	// MSAL returns its AuthResult. Keep the opaque export in memory, then commit
	// it immediately once the provider has verified that result.
	if a.binding.HomeAccountID == "" {
		if !a.allowCreate || a.secretID != "" {
			return managedError(ManagedCacheInvalid, "stage managed cache", nil)
		}
		a.stagedCache = bytes.Clone(raw)
		return nil
	}

	return a.persist(ctx, raw)
}

func (a *managedCacheAdapter) commitInitial(ctx context.Context, homeAccountID string) error {
	if homeAccountID == "" || len(a.stagedCache) == 0 || a.secretID != "" {
		return managedError(ManagedCacheInvalid, "commit initial managed cache", nil)
	}
	a.binding.HomeAccountID = homeAccountID
	err := a.persist(ctx, a.stagedCache)
	clear(a.stagedCache)
	a.stagedCache = nil
	return err
}

func (a *managedCacheAdapter) persist(ctx context.Context, raw []byte) error {
	ctx, cancel := managedOperationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !a.binding.valid() || !json.Valid(raw) {
		return managedError(ManagedCacheInvalid, "persist managed cache", nil)
	}

	var previousDigest *string
	nextGeneration := uint64(1)
	key := managedSecretKey(a.binding)
	if a.secretID != "" {
		secret, current, digest, err := a.readAndValidate(ctx, a.secretID)
		if err != nil {
			return err
		}
		if current.Generation != a.loadedGeneration || digest != a.loadedDigest {
			return managedError(ManagedCacheConflict, "detect stale managed cache", nil)
		}
		key = secret.Key
		nextGeneration = current.Generation + 1
		previousDigest = &digest
	}

	envelope := managedEnvelope{
		SchemaVersion:  managedCacheSchema,
		Binding:        a.binding,
		Generation:     nextGeneration,
		PreviousDigest: previousDigest,
		Cache:          bytes.Clone(raw),
		UpdatedAt:      a.now().UTC().Format(time.RFC3339Nano),
		Writer:         a.host,
	}
	encoded, err := marshalManagedEnvelope(envelope)
	if err != nil {
		return managedError(ManagedCacheInvalid, "serialise managed cache", err)
	}

	var written *managedSecret
	if a.secretID == "" {
		written, err = a.store.Create(ctx, key, string(encoded), a.organisationID, a.projectID)
	} else {
		written, err = a.store.Update(ctx, a.secretID, key, string(encoded), a.organisationID, a.projectID)
	}
	if err != nil {
		if contextError(ctx) != nil {
			return contextError(ctx)
		}
		return managedError(ManagedCacheUnavailable, "write managed cache", err)
	}
	if written == nil || written.ID == "" {
		return managedError(ManagedCacheUnavailable, "write managed cache", nil)
	}
	a.secretID = written.ID

	_, readback, readbackDigest, err := a.readAndValidate(ctx, a.secretID)
	if err != nil {
		return err
	}
	expectedDigest := digestCache(raw)
	if readback.Generation != nextGeneration || readbackDigest != expectedDigest ||
		!equalOptionalDigest(readback.PreviousDigest, previousDigest) || readback.Writer != a.host {
		return managedError(ManagedCacheConflict, "verify managed cache readback", nil)
	}
	a.loadedGeneration = nextGeneration
	a.loadedDigest = expectedDigest
	return nil
}

func (a *managedCacheAdapter) readAndValidate(ctx context.Context, secretID string) (*managedSecret, managedEnvelope, string, error) {
	secret, err := a.store.Get(ctx, secretID)
	if err != nil {
		if contextError(ctx) != nil {
			return nil, managedEnvelope{}, "", contextError(ctx)
		}
		return nil, managedEnvelope{}, "", managedError(ManagedCacheUnavailable, "read managed cache", err)
	}
	if secret == nil || secret.ID != secretID || secret.OrganisationID != a.organisationID || secret.ProjectID != a.projectID {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "validate managed cache location", nil)
	}

	envelope, err := unmarshalManagedEnvelope([]byte(secret.Value))
	if err != nil {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "parse managed cache", err)
	}
	if envelope.SchemaVersion != managedCacheSchema || envelope.Binding != a.binding ||
		envelope.Generation == 0 || envelope.Writer != a.host {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "validate managed cache binding", nil)
	}
	if envelope.PreviousDigest != nil && !digestPattern.MatchString(*envelope.PreviousDigest) {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "validate managed cache digest", nil)
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.UpdatedAt); err != nil {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "validate managed cache timestamp", err)
	}
	if !json.Valid(envelope.Cache) {
		return nil, managedEnvelope{}, "", managedError(ManagedCacheInvalid, "validate managed cache payload", nil)
	}
	return secret, envelope, digestCache(envelope.Cache), nil
}

func (a *managedCacheAdapter) deleteAndVerify(ctx context.Context) error {
	if a.secretID == "" {
		return nil
	}
	if err := a.store.Delete(ctx, a.secretID); err != nil {
		return managedError(ManagedCacheUnavailable, "delete managed cache", err)
	}
	_, err := a.store.Get(ctx, a.secretID)
	if !errors.Is(err, errManagedSecretNotFound) {
		return managedError(ManagedCacheConflict, "verify managed cache deletion", nil)
	}
	return nil
}

func marshalManagedEnvelope(envelope managedEnvelope) ([]byte, error) {
	if !json.Valid(envelope.Cache) {
		return nil, errors.New("cache is not a complete JSON value")
	}
	expectedDigest := digestCache(envelope.Cache)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	decoded, err := unmarshalManagedEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if digestCache(decoded.Cache) != expectedDigest {
		return nil, errors.New("cache bytes changed during serialisation")
	}
	return encoded, nil
}

func unmarshalManagedEnvelope(data []byte) (managedEnvelope, error) {
	var envelope managedEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return managedEnvelope{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return managedEnvelope{}, errors.New("multiple JSON values")
		}
		return managedEnvelope{}, err
	}
	return envelope, nil
}

func managedSecretKey(binding managedBinding) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		binding.TenantID,
		binding.ClientID,
		binding.HomeAccountID,
		binding.Profile,
	}, "\x00")))
	return "cb365-delegated-cache-" + hex.EncodeToString(sum[:16])
}

func digestCache(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func equalOptionalDigest(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func managedOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 30*time.Second)
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (a *managedCacheAdapter) String() string {
	return fmt.Sprintf("managed cache adapter for profile %q", a.binding.Profile)
}
