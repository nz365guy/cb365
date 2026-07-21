//go:build linux && cgo

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/nz365guy/cb365/internal/config"
)

type fakeProvenanceStore struct{ *fakeManagedStore }

func (s *fakeProvenanceStore) CreateProvenance(ctx context.Context, key, value, organisationID, projectID string) (*managedSecret, error) {
	return s.Create(ctx, key, value, organisationID, projectID)
}

func testManagedProvenanceProfile() *config.Profile {
	return &config.Profile{
		Name:     "profile",
		TenantID: "tenant",
		ClientID: "client",
		AuthMode: config.AuthModeDelegated,
		ManagedDelegated: &config.ManagedDelegatedMetadata{
			HomeAccountID:  "home",
			SecretID:       "cache-secret",
			OrganisationID: "org",
			ProjectID:      "project",
			AssignedHost:   "host",
			MigrationState: managedMigrationComplete,
		},
	}
}

func TestManagedChannelMessageProvenanceStoreRoundTripAndExactBinding(t *testing.T) {
	store := &fakeProvenanceStore{fakeManagedStore: newFakeManagedStore()}
	profile := testManagedProvenanceProfile()
	binding, err := managedProfileBinding(profile)
	if err != nil {
		t.Fatal(err)
	}
	target := ManagedChannelMessageTarget{TeamID: "team", ChannelID: "channel", MessageID: "message"}
	reference, err := recordManagedChannelMessageProvenance(
		context.Background(),
		store,
		profile,
		binding,
		target,
		"host",
		func() time.Time { return time.Date(2026, 7, 21, 11, 0, 0, 0, time.UTC) },
	)
	if err != nil || reference == "" {
		t.Fatalf("record: reference=%q err=%v", reference, err)
	}
	if err := verifyManagedChannelMessageProvenance(context.Background(), store, profile, binding, reference, target, "host"); err != nil {
		t.Fatalf("verify: %v", err)
	}

	tests := []struct {
		name    string
		binding managedBinding
		target  ManagedChannelMessageTarget
		host    string
	}{
		{name: "tenant", binding: managedBinding{TenantID: "other", ClientID: binding.ClientID, HomeAccountID: binding.HomeAccountID, Profile: binding.Profile}, target: target, host: "host"},
		{name: "client", binding: managedBinding{TenantID: binding.TenantID, ClientID: "other", HomeAccountID: binding.HomeAccountID, Profile: binding.Profile}, target: target, host: "host"},
		{name: "account", binding: managedBinding{TenantID: binding.TenantID, ClientID: binding.ClientID, HomeAccountID: "other", Profile: binding.Profile}, target: target, host: "host"},
		{name: "profile", binding: managedBinding{TenantID: binding.TenantID, ClientID: binding.ClientID, HomeAccountID: binding.HomeAccountID, Profile: "other"}, target: target, host: "host"},
		{name: "team", binding: binding, target: ManagedChannelMessageTarget{TeamID: "other", ChannelID: target.ChannelID, MessageID: target.MessageID}, host: "host"},
		{name: "channel", binding: binding, target: ManagedChannelMessageTarget{TeamID: target.TeamID, ChannelID: "other", MessageID: target.MessageID}, host: "host"},
		{name: "message", binding: binding, target: ManagedChannelMessageTarget{TeamID: target.TeamID, ChannelID: target.ChannelID, MessageID: "other"}, host: "host"},
		{name: "host", binding: binding, target: target, host: "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := verifyManagedChannelMessageProvenance(context.Background(), store, profile, test.binding, reference, test.target, test.host); err == nil {
				t.Fatal("mismatched provenance binding was accepted")
			}
		})
	}
}
