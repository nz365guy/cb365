package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMailDeltaStateRoundTripIsFolderScopedAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")
	want := mailDeltaState{
		Version: 1,
		Folders: map[string]mailDeltaFolderState{
			"inbox": {DeltaLink: "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages/delta?$deltatoken=opaque"},
		},
	}
	if err := saveMailDeltaState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadMailDeltaState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Folders["inbox"].DeltaLink != want.Folders["inbox"].DeltaLink {
		t.Fatalf("delta link = %q, want %q", got.Folders["inbox"].DeltaLink, want.Folders["inbox"].DeltaLink)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
}

func TestMailDeltaTokenExpiredRecognisesGraphRecoverySignals(t *testing.T) {
	for _, detail := range []string{"ErrorInvalidDateTimeToken", "SyncStateNotFound", "HTTP status code 410"} {
		if !mailDeltaTokenExpired(assertionError(detail)) {
			t.Fatalf("expected %q to require full resynchronisation", detail)
		}
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
