package auth

import (
	"strings"
	"testing"
	"time"
)

func TestManagedChannelMessageProvenanceRoundTripAndUnknownFieldRejection(t *testing.T) {
	envelope := managedChannelMessageProvenance{
		SchemaVersion: managedChannelMessageProvenanceSchema,
		Binding: managedBinding{
			TenantID:      "tenant",
			ClientID:      "client",
			HomeAccountID: "account",
			Profile:       "profile",
		},
		Target: ManagedChannelMessageTarget{
			TeamID:    "team",
			ChannelID: "channel",
			MessageID: "message",
		},
		CreatedAt: time.Date(2026, 7, 21, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Writer:    "host",
	}
	encoded, err := marshalManagedChannelMessageProvenance(envelope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := unmarshalManagedChannelMessageProvenance(encoded)
	if err != nil || decoded != envelope {
		t.Fatalf("round trip: decoded=%+v err=%v", decoded, err)
	}
	withUnknown := strings.TrimSuffix(string(encoded), "}") + `,"secretValue":"forbidden"}`
	if _, err := unmarshalManagedChannelMessageProvenance([]byte(withUnknown)); err == nil {
		t.Fatal("unknown secret-bearing field was accepted")
	}
}

func TestManagedChannelMessageProvenanceKeysBindEveryField(t *testing.T) {
	target := ManagedChannelMessageTarget{TeamID: "team", ChannelID: "channel", MessageID: "message"}
	key := ManagedChannelMessageProvenanceKey(target)
	if !strings.HasPrefix(key, "sha256:") || len(key) != len("sha256:")+64 {
		t.Fatalf("unexpected lookup key %q", key)
	}
	variants := []ManagedChannelMessageTarget{
		{TeamID: "other", ChannelID: target.ChannelID, MessageID: target.MessageID},
		{TeamID: target.TeamID, ChannelID: "other", MessageID: target.MessageID},
		{TeamID: target.TeamID, ChannelID: target.ChannelID, MessageID: "other"},
	}
	for _, variant := range variants {
		if ManagedChannelMessageProvenanceKey(variant) == key {
			t.Fatalf("variant did not change key: %+v", variant)
		}
	}
}
