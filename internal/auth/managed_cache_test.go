package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
)

var managedTestSentinel = "e" + "yJhbGci" + "OiJub25lIn0" + ".opaque-managed-test-sentinel"

type fakeManagedStore struct {
	secrets       map[string]*managedSecret
	nextID        int
	updates       int
	mutateOnWrite func(*managedSecret)
}

func newFakeManagedStore() *fakeManagedStore {
	return &fakeManagedStore{secrets: make(map[string]*managedSecret)}
}

func (s *fakeManagedStore) Get(_ context.Context, id string) (*managedSecret, error) {
	secret, ok := s.secrets[id]
	if !ok {
		return nil, errManagedSecretNotFound
	}
	copy := *secret
	return &copy, nil
}

func (s *fakeManagedStore) Create(_ context.Context, key, value, organisationID, projectID string) (*managedSecret, error) {
	s.nextID++
	secret := &managedSecret{ID: "secret-" + string(rune('0'+s.nextID)), Key: key, Value: value, OrganisationID: organisationID, ProjectID: projectID}
	if s.mutateOnWrite != nil {
		s.mutateOnWrite(secret)
	}
	s.secrets[secret.ID] = secret
	copy := *secret
	return &copy, nil
}

func (s *fakeManagedStore) Update(_ context.Context, id, key, value, organisationID, projectID string) (*managedSecret, error) {
	if _, ok := s.secrets[id]; !ok {
		return nil, errManagedSecretNotFound
	}
	s.updates++
	secret := &managedSecret{ID: id, Key: key, Value: value, OrganisationID: organisationID, ProjectID: projectID}
	if s.mutateOnWrite != nil {
		s.mutateOnWrite(secret)
	}
	s.secrets[id] = secret
	copy := *secret
	return &copy, nil
}

func (s *fakeManagedStore) Delete(_ context.Context, id string) error {
	delete(s.secrets, id)
	return nil
}

type fakeMarshaler struct {
	raw []byte
	err error
}

func (m fakeMarshaler) Marshal() ([]byte, error) { return bytes.Clone(m.raw), m.err }

type fakeUnmarshaler struct{ raw []byte }

func (u *fakeUnmarshaler) Unmarshal(raw []byte) error {
	u.raw = bytes.Clone(raw)
	return nil
}

func testManagedBinding() managedBinding {
	return managedBinding{TenantID: "tenant", ClientID: "client", HomeAccountID: "home", Profile: "profile"}
}

func testManagedAdapter(t *testing.T, store *fakeManagedStore, binding managedBinding, secretID string, allowCreate bool) *managedCacheAdapter {
	t.Helper()
	adapter, err := newManagedCacheAdapter(store, binding, "org", "project", secretID, "host", allowCreate)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestManagedCacheCreateReplaceAndMonotonicUpdate(t *testing.T) {
	ctx := context.Background()
	store := newFakeManagedStore()
	adapter := testManagedAdapter(t, store, testManagedBinding(), "", true)
	first := []byte(`{"AccessToken":{"one":"value"}}`)
	if err := adapter.Export(ctx, fakeMarshaler{raw: first}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	if adapter.secretID == "" {
		t.Fatal("first export did not create a secret reference")
	}
	secret := store.secrets[adapter.secretID]
	envelope, err := unmarshalManagedEnvelope([]byte(secret.Value))
	if err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != managedCacheSchema || envelope.Generation != 1 || envelope.PreviousDigest != nil || !bytes.Equal(envelope.Cache, first) {
		t.Fatalf("unexpected first envelope metadata")
	}

	reader := testManagedAdapter(t, store, testManagedBinding(), adapter.secretID, false)
	target := &fakeUnmarshaler{}
	if err := reader.Replace(ctx, target, msalcache.ReplaceHints{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(target.raw, first) {
		t.Fatal("replace changed opaque cache bytes")
	}
	second := []byte(`{"AccessToken":{"two":"value"}}`)
	if err := reader.Export(ctx, fakeMarshaler{raw: second}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	envelope, err = unmarshalManagedEnvelope([]byte(store.secrets[adapter.secretID].Value))
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Generation != 2 || envelope.PreviousDigest == nil || *envelope.PreviousDigest != digestCache(first) || !bytes.Equal(envelope.Cache, second) {
		t.Fatal("update did not preserve generation and previous digest invariants")
	}
}

func TestManagedCacheRejectsStaleWriterBeforeUpdate(t *testing.T) {
	ctx := context.Background()
	store := newFakeManagedStore()
	creator := testManagedAdapter(t, store, testManagedBinding(), "", true)
	if err := creator.Export(ctx, fakeMarshaler{raw: []byte(`{"v":1}`)}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	left := testManagedAdapter(t, store, testManagedBinding(), creator.secretID, false)
	right := testManagedAdapter(t, store, testManagedBinding(), creator.secretID, false)
	if err := left.Replace(ctx, &fakeUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		t.Fatal(err)
	}
	if err := right.Replace(ctx, &fakeUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		t.Fatal(err)
	}
	if err := left.Export(ctx, fakeMarshaler{raw: []byte(`{"v":2}`)}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	updates := store.updates
	err := right.Export(ctx, fakeMarshaler{raw: []byte(`{"v":3}`)}, msalcache.ExportHints{})
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheConflict {
		t.Fatalf("expected managed cache conflict, got %v", err)
	}
	if store.updates != updates {
		t.Fatal("stale writer reached the store update")
	}
}

func TestManagedCacheRejectsBindingSchemaAndReadbackChanges(t *testing.T) {
	ctx := context.Background()
	store := newFakeManagedStore()
	creator := testManagedAdapter(t, store, testManagedBinding(), "", true)
	if err := creator.Export(ctx, fakeMarshaler{raw: []byte(`{"v":1}`)}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}

	wrong := testManagedBinding()
	wrong.TenantID = "other"
	err := testManagedAdapter(t, store, wrong, creator.secretID, false).Replace(ctx, &fakeUnmarshaler{}, msalcache.ReplaceHints{})
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheInvalid {
		t.Fatalf("expected invalid binding, got %v", err)
	}

	secret := store.secrets[creator.secretID]
	envelope, err := unmarshalManagedEnvelope([]byte(secret.Value))
	if err != nil {
		t.Fatal(err)
	}
	envelope.SchemaVersion = "cb365.msal-cache/v1"
	encoded, err := marshalManagedEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	secret.Value = string(encoded)
	err = testManagedAdapter(t, store, testManagedBinding(), creator.secretID, false).Replace(ctx, &fakeUnmarshaler{}, msalcache.ReplaceHints{})
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheInvalid {
		t.Fatalf("expected unknown schema rejection, got %v", err)
	}

	// Restore a valid record, load it, then alter the just-written readback.
	envelope.SchemaVersion = managedCacheSchema
	encoded, _ = marshalManagedEnvelope(envelope)
	secret.Value = string(encoded)
	writer := testManagedAdapter(t, store, testManagedBinding(), creator.secretID, false)
	if err := writer.Replace(ctx, &fakeUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		t.Fatal(err)
	}
	store.mutateOnWrite = func(secret *managedSecret) {
		var changed managedEnvelope
		_ = jsonUnmarshalNoSecret([]byte(secret.Value), &changed)
		changed.Generation++
		mutated, _ := marshalManagedEnvelope(changed)
		secret.Value = string(mutated)
	}
	err = writer.Export(ctx, fakeMarshaler{raw: []byte(`{"v":2}`)}, msalcache.ExportHints{})
	if class, ok := ManagedErrorClassOf(err); !ok || class != ManagedCacheConflict {
		t.Fatalf("expected readback conflict, got %v", err)
	}
}

func TestManagedCacheDeleteAndRedactedErrors(t *testing.T) {
	ctx := context.Background()
	store := newFakeManagedStore()
	adapter := testManagedAdapter(t, store, testManagedBinding(), "", true)
	if err := adapter.Export(ctx, fakeMarshaler{raw: []byte(`{"v":1}`)}, msalcache.ExportHints{}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.deleteAndVerify(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.secrets[adapter.secretID]; ok {
		t.Fatal("secret remained after verified delete")
	}

	redaction := testManagedAdapter(t, newFakeManagedStore(), testManagedBinding(), "", true)
	err := redaction.Export(ctx, fakeMarshaler{err: errors.New(managedTestSentinel)}, msalcache.ExportHints{})
	if err == nil || strings.Contains(err.Error(), managedTestSentinel) || strings.Contains(redaction.String(), managedTestSentinel) {
		t.Fatalf("managed error or adapter string exposed sentinel")
	}
}

func TestManagedEnvelopeRejectsByteChangingAndUnknownJSON(t *testing.T) {
	envelope := managedEnvelope{
		SchemaVersion: managedCacheSchema,
		Binding:       testManagedBinding(),
		Generation:    1,
		Cache:         []byte("{ \"v\" : 1 }"),
		UpdatedAt:     "2026-07-21T00:00:00Z",
		Writer:        "host",
	}
	if _, err := marshalManagedEnvelope(envelope); err == nil {
		t.Fatal("expected whitespace-normalising serialisation to fail byte-preservation check")
	}
	if _, err := unmarshalManagedEnvelope([]byte(`{"schemaVersion":"cb365.msal-cache/v2","unknown":true}`)); err == nil {
		t.Fatal("unknown envelope fields were accepted")
	}
}

// Keep encoding/json use behind a tiny helper so test failures never format
// the bearer-shaped fixture.
func jsonUnmarshalNoSecret(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	return decoder.Decode(target)
}
