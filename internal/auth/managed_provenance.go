package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
)

const managedChannelMessageProvenanceSchema = "cb365.teams-channel-message-provenance/v1"

// ManagedChannelMessageTarget identifies one root Teams channel message.
// It intentionally contains identifiers only, never message content.
type ManagedChannelMessageTarget struct {
	TeamID    string `json:"teamId"`
	ChannelID string `json:"channelId"`
	MessageID string `json:"messageId"`
}

func (t ManagedChannelMessageTarget) valid() bool {
	for _, value := range []string{t.TeamID, t.ChannelID, t.MessageID} {
		if value == "" || len(value) > 512 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return false
		}
	}
	return true
}

type managedChannelMessageProvenance struct {
	SchemaVersion string                      `json:"schemaVersion"`
	Binding       managedBinding              `json:"binding"`
	Target        ManagedChannelMessageTarget `json:"target"`
	CreatedAt     string                      `json:"createdAt"`
	Writer        string                      `json:"writer"`
}

// ManagedChannelMessageProvenanceKey returns the non-secret local lookup key
// for a BWS provenance record reference.
func ManagedChannelMessageProvenanceKey(target ManagedChannelMessageTarget) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		target.TeamID,
		target.ChannelID,
		target.MessageID,
	}, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func managedChannelMessageSecretKey(binding managedBinding, target ManagedChannelMessageTarget) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		binding.TenantID,
		binding.ClientID,
		binding.HomeAccountID,
		binding.Profile,
		target.TeamID,
		target.ChannelID,
		target.MessageID,
	}, "\x00")))
	return "cb365-teams-message-" + hex.EncodeToString(sum[:16])
}

func marshalManagedChannelMessageProvenance(envelope managedChannelMessageProvenance) ([]byte, error) {
	if envelope.SchemaVersion != managedChannelMessageProvenanceSchema ||
		!envelope.Binding.valid() || !envelope.Target.valid() || envelope.Writer == "" {
		return nil, errors.New("invalid channel-message provenance")
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.CreatedAt); err != nil {
		return nil, errors.New("invalid channel-message provenance timestamp")
	}
	return json.Marshal(envelope)
}

func unmarshalManagedChannelMessageProvenance(data []byte) (managedChannelMessageProvenance, error) {
	var envelope managedChannelMessageProvenance
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return managedChannelMessageProvenance{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return managedChannelMessageProvenance{}, errors.New("multiple JSON values")
		}
		return managedChannelMessageProvenance{}, err
	}
	if _, err := marshalManagedChannelMessageProvenance(envelope); err != nil {
		return managedChannelMessageProvenance{}, err
	}
	return envelope, nil
}
