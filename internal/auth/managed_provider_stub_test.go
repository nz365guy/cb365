//go:build !linux || !cgo

package auth

import (
	"context"
	"testing"

	"github.com/nz365guy/cb365/internal/config"
)

func TestManagedDelegatedStubFailsClosed(t *testing.T) {
	if ManagedDelegatedAvailable() {
		t.Fatal("unsupported build reported managed provider available")
	}
	profile := &config.Profile{Name: "profile", TenantID: "tenant", ClientID: "client", AuthMode: config.AuthModeDelegated}
	if _, err := NewManagedDelegatedCredential(profile, false); err == nil || err.Error() != "managed delegated authentication unavailable on this build" {
		t.Fatalf("unexpected stub error: %v", err)
	}
	if err := DeleteManagedDelegated(context.Background(), profile); err == nil || err.Error() != "managed delegated authentication unavailable on this build" {
		t.Fatalf("unexpected delete stub error: %v", err)
	}
	target := ManagedChannelMessageTarget{TeamID: "team", ChannelID: "channel", MessageID: "message"}
	if _, err := RecordManagedChannelMessageProvenance(context.Background(), profile, target); err == nil || err.Error() != "managed delegated authentication unavailable on this build" {
		t.Fatalf("unexpected provenance record stub error: %v", err)
	}
	if err := VerifyManagedChannelMessageProvenance(context.Background(), profile, "reference", target); err == nil || err.Error() != "managed delegated authentication unavailable on this build" {
		t.Fatalf("unexpected provenance verify stub error: %v", err)
	}
	if err := DeleteManagedChannelMessageProvenance(context.Background(), profile, "reference", target); err == nil || err.Error() != "managed delegated authentication unavailable on this build" {
		t.Fatalf("unexpected provenance delete stub error: %v", err)
	}
}
