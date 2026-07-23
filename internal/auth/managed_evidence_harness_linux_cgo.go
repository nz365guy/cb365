//go:build linux && cgo && managed_evidence

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
)

// ManagedEvidenceResult is secret-free evidence returned only by the
// managed_evidence integration build. It is not compiled into release builds.
type ManagedEvidenceResult struct {
	Class            ManagedErrorClass
	EntraRequests    int
	WorkloadRequests int
	PromptCalls      int
	LegacyReads      int
	FinalGeneration  uint64
	StoreUpdates     int
	SourceRetained   bool
	SentinelResidue  bool
	History          []string
}

type evidenceStore struct {
	mu              sync.Mutex
	secrets         map[string]*managedSecret
	nextID          int
	updates         int
	getErr          error
	corruptReadback bool
	history         []string
}

func newEvidenceStore() *evidenceStore {
	return &evidenceStore{secrets: make(map[string]*managedSecret)}
}

func (s *evidenceStore) Get(_ context.Context, id string) (*managedSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, "get:"+id)
	if s.getErr != nil {
		return nil, s.getErr
	}
	secret, ok := s.secrets[id]
	if !ok {
		return nil, errManagedSecretNotFound
	}
	copy := *secret
	if s.corruptReadback {
		var envelope managedEnvelope
		if err := json.Unmarshal([]byte(copy.Value), &envelope); err != nil {
			return nil, err
		}
		envelope.Generation++
		encoded, err := marshalManagedEnvelope(envelope)
		if err != nil {
			return nil, err
		}
		copy.Value = string(encoded)
	}
	return &copy, nil
}

func (s *evidenceStore) Create(_ context.Context, key, value, organisationID, projectID string) (*managedSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := fmt.Sprintf("secret-%d", s.nextID)
	s.history = append(s.history, "create:"+id)
	secret := &managedSecret{ID: id, Key: key, Value: value, OrganisationID: organisationID, ProjectID: projectID}
	s.secrets[id] = secret
	copy := *secret
	return &copy, nil
}

func (s *evidenceStore) Update(_ context.Context, id, key, value, organisationID, projectID string) (*managedSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.secrets[id]; !ok {
		return nil, errManagedSecretNotFound
	}
	s.updates++
	s.history = append(s.history, "update:"+id)
	secret := &managedSecret{ID: id, Key: key, Value: value, OrganisationID: organisationID, ProjectID: projectID}
	s.secrets[id] = secret
	copy := *secret
	return &copy, nil
}

func (s *evidenceStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, "delete:"+id)
	delete(s.secrets, id)
	return nil
}

type evidenceMarshaler []byte

func (m evidenceMarshaler) Marshal() ([]byte, error) { return bytes.Clone(m), nil }

type evidenceUnmarshaler struct{}

func (*evidenceUnmarshaler) Unmarshal([]byte) error { return nil }

func evidenceBinding() managedBinding {
	return managedBinding{TenantID: "tenant", ClientID: "client", HomeAccountID: "home", Profile: "evidence-profile"}
}

func evidenceAdapter(store *evidenceStore, secretID string, allowCreate bool) (*managedCacheAdapter, error) {
	return newManagedCacheAdapter(store, evidenceBinding(), "organisation", "project", secretID, "evidence-host", allowCreate)
}

func evidenceClass(err error) (ManagedErrorClass, error) {
	class, ok := ManagedErrorClassOf(err)
	if !ok {
		return "", fmt.Errorf("managed evidence returned an untyped error")
	}
	return class, nil
}

// RunManagedEvidenceSuite executes the credential-free T3-T6 evidence matrix.
// It uses only in-memory fake BWS state and an isolated caller-owned directory.
func RunManagedEvidenceSuite(root string) (map[string]ManagedEvidenceResult, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("managed evidence root must be absolute")
	}
	results := make(map[string]ManagedEvidenceResult)
	if err := runEvidenceT3(results); err != nil {
		return nil, err
	}
	sentinel := "e" + "yJhbGci" + "OiJub25lIn0" + ".opaque-managed-evidence-sentinel"
	if err := runEvidenceT4(results, root, sentinel); err != nil {
		return nil, err
	}
	if err := runEvidenceT5(results, root); err != nil {
		return nil, err
	}
	if err := runEvidenceT6(results, root, sentinel); err != nil {
		return nil, err
	}
	return results, nil
}

func runEvidenceT3(results map[string]ManagedEvidenceResult) error {
	for _, scenario := range []struct {
		name   string
		getErr error
	}{
		{name: "t3a"},
		{name: "t3b", getErr: errors.New("provider detail must stay redacted")},
	} {
		store := newEvidenceStore()
		store.getErr = scenario.getErr
		adapter, err := evidenceAdapter(store, "missing-record", false)
		if err != nil {
			return err
		}
		err = adapter.Replace(context.Background(), &evidenceUnmarshaler{}, msalcache.ReplaceHints{})
		class, classErr := evidenceClass(err)
		if classErr != nil {
			return classErr
		}
		results[scenario.name] = ManagedEvidenceResult{Class: class, History: append([]string(nil), store.history...)}
	}
	return nil
}

func runEvidenceT4(results map[string]ManagedEvidenceResult, root, sentinel string) error {
	store := newEvidenceStore()
	adapter, err := evidenceAdapter(store, "", true)
	if err != nil {
		return err
	}
	raw := []byte(`{"AccessToken":{"fixture":"` + sentinel + `"}}`)
	if err := adapter.Export(context.Background(), evidenceMarshaler(raw), msalcache.ExportHints{}); err != nil {
		return err
	}
	contained := 0
	for _, secret := range store.secrets {
		if strings.Contains(secret.Value, sentinel) {
			contained++
		}
	}
	if contained != 1 || strings.Contains(strings.Join(store.history, "\n"), sentinel) {
		return errors.New("sentinel escaped the single fake record")
	}
	clear(store.secrets)
	artifact := filepath.Join(root, "t4-evidence.json")
	if err := os.WriteFile(artifact, []byte(`{"result":"pass"}`), 0600); err != nil {
		return err
	}
	residue, err := scanEvidenceRoot(root, sentinel)
	if err != nil {
		return err
	}
	results["t4"] = ManagedEvidenceResult{SentinelResidue: residue, History: append([]string(nil), store.history...)}
	return nil
}

func runEvidenceT5(results map[string]ManagedEvidenceResult, root string) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		return err
	}
	previous, hadPrevious := os.LookupEnv("XDG_RUNTIME_DIR")
	if err := os.Setenv("XDG_RUNTIME_DIR", runtimeDir); err != nil {
		return err
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv("XDG_RUNTIME_DIR", previous)
		} else {
			_ = os.Unsetenv("XDG_RUNTIME_DIR")
		}
	}()

	winner, err := acquireManagedProfileLock("evidence-profile", "evidence-host")
	if err != nil {
		return err
	}
	_, contenderErr := acquireManagedProfileLock("evidence-profile", "evidence-host")
	class, err := evidenceClass(contenderErr)
	if err != nil {
		_ = winner.Close()
		return err
	}
	if err := winner.Close(); err != nil {
		return err
	}

	store := newEvidenceStore()
	creator, err := evidenceAdapter(store, "", true)
	if err != nil {
		return err
	}
	if err := creator.Export(context.Background(), evidenceMarshaler(`{"v":1}`), msalcache.ExportHints{}); err != nil {
		return err
	}
	writer, err := evidenceAdapter(store, creator.secretID, false)
	if err != nil {
		return err
	}
	if err := writer.Replace(context.Background(), &evidenceUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		return err
	}
	if err := writer.Export(context.Background(), evidenceMarshaler(`{"v":2}`), msalcache.ExportHints{}); err != nil {
		return err
	}
	_, envelope, _, err := writer.readAndValidate(context.Background(), writer.secretID)
	if err != nil {
		return err
	}
	results["t5a"] = ManagedEvidenceResult{Class: class, FinalGeneration: envelope.Generation, StoreUpdates: store.updates, History: append([]string(nil), store.history...)}

	left, _ := evidenceAdapter(store, writer.secretID, false)
	right, _ := evidenceAdapter(store, writer.secretID, false)
	if err := left.Replace(context.Background(), &evidenceUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		return err
	}
	if err := right.Replace(context.Background(), &evidenceUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
		return err
	}
	if err := left.Export(context.Background(), evidenceMarshaler(`{"v":3}`), msalcache.ExportHints{}); err != nil {
		return err
	}
	updates := store.updates
	staleErr := right.Export(context.Background(), evidenceMarshaler(`{"v":4}`), msalcache.ExportHints{})
	staleClass, err := evidenceClass(staleErr)
	if err != nil {
		return err
	}
	_, final, _, err := left.readAndValidate(context.Background(), left.secretID)
	if err != nil {
		return err
	}
	results["t5b"] = ManagedEvidenceResult{Class: staleClass, FinalGeneration: final.Generation, StoreUpdates: store.updates - updates, History: append([]string(nil), store.history...)}
	return nil
}

func runEvidenceT6(results map[string]ManagedEvidenceResult, root, sentinel string) error {
	legacyDir := filepath.Join(root, "legacy")
	if err := os.Mkdir(legacyDir, 0700); err != nil {
		return err
	}
	legacyFile := filepath.Join(legacyDir, "tokens.enc")
	if err := os.WriteFile(legacyFile, []byte("encrypted legacy fixture"), 0600); err != nil {
		return err
	}
	history := []string{"permission-check"}
	if err := verifyOptionalOwnedPath(legacyDir, true, 0700, true); err != nil {
		return err
	}
	if err := verifyOptionalOwnedPath(legacyFile, false, 0600, true); err != nil {
		return err
	}
	history = append(history, "legacy-read")
	if _, err := os.ReadFile(legacyFile); err != nil { // #nosec G304 -- test-only path is derived from the caller-owned absolute temporary root
		return err
	}
	store := newEvidenceStore()
	adapter, err := evidenceAdapter(store, "", true)
	if err != nil {
		return err
	}
	if err := adapter.Export(context.Background(), evidenceMarshaler([]byte(`{"AccessToken":{"fixture":"`+sentinel+`"}}`)), msalcache.ExportHints{}); err != nil {
		return err
	}
	history = append(history, store.history...)
	if err := os.Remove(legacyFile); err != nil {
		return err
	}
	history = append(history, "legacy-delete")
	results["t6a"] = ManagedEvidenceResult{LegacyReads: 1, SourceRetained: fileExists(legacyFile), History: append([]string(nil), history...)}

	if err := os.WriteFile(legacyFile, []byte("encrypted legacy fixture"), 0644); err != nil { // #nosec G306 -- deliberately unsafe mode fixture must be rejected before read
		return err
	}
	permissionErr := verifyOptionalOwnedPath(legacyFile, false, 0600, true)
	permissionClass, err := evidenceClass(permissionErr)
	if err != nil {
		return err
	}
	results["t6b-permission"] = ManagedEvidenceResult{Class: permissionClass, LegacyReads: 0, SourceRetained: fileExists(legacyFile)}
	if err := os.Chmod(legacyFile, 0600); err != nil {
		return err
	}
	failedStore := newEvidenceStore()
	failedStore.corruptReadback = true
	failedAdapter, err := evidenceAdapter(failedStore, "", true)
	if err != nil {
		return err
	}
	readbackErr := failedAdapter.Export(context.Background(), evidenceMarshaler(`{"v":1}`), msalcache.ExportHints{})
	readbackClass, err := evidenceClass(readbackErr)
	if err != nil {
		return err
	}
	results["t6b-readback"] = ManagedEvidenceResult{Class: readbackClass, LegacyReads: 1, SourceRetained: fileExists(legacyFile), History: append([]string(nil), failedStore.history...)}

	if err := adapter.deleteAndVerify(context.Background()); err != nil {
		return err
	}
	if err := os.Remove(legacyFile); err != nil {
		return err
	}
	residue, err := scanEvidenceRoot(root, sentinel)
	if err != nil {
		return err
	}
	results["t6c"] = ManagedEvidenceResult{SourceRetained: fileExists(legacyFile), SentinelResidue: residue, History: append([]string(nil), store.history...)}
	return nil
}

func scanEvidenceRoot(root, sentinel string) (bool, error) {
	jwtPrefix := "e" + "yJ"
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G122 G304 -- test-only scanner walks a non-concurrent t.TempDir-owned root
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(sentinel)) || bytes.Contains(data, []byte(jwtPrefix)) {
			return errEvidenceResidue
		}
		return nil
	})
	if errors.Is(err, errEvidenceResidue) {
		return true, nil
	}
	return false, err
}

var errEvidenceResidue = errors.New("managed evidence residue found")

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
