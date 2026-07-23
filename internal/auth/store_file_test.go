package auth

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestFileStoreBindsCiphertextToProfile(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	want := []byte(`{"access_token":"synthetic"}`)
	if err := backend.Set("profile-a", want); err != nil {
		t.Fatal(err)
	}
	got, err := backend.Get("profile-a")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("round trip failed: got %q, err %v", got, err)
	}

	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Entries["profile-b"] = append([]byte(nil), store.Entries["profile-a"]...)
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get("profile-b"); err == nil {
		t.Fatal("ciphertext moved to another profile label must not decrypt")
	}
}

func TestFileStoreRejectsLegacyUnboundEntries(t *testing.T) {
	backend := &fileBackend{path: filepath.Join(t.TempDir(), fileStoreName), password: "synthetic-test-password"}
	store, err := backend.load()
	if err != nil {
		t.Fatal(err)
	}
	store.Version = 0
	store.Entries["legacy"] = []byte("synthetic-ciphertext")
	if err := backend.save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get("legacy"); err == nil {
		t.Fatal("legacy unbound entry must require reauthentication")
	}
}
