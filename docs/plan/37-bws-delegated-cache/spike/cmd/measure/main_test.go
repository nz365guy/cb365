package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestRawMessagePreservesAlreadyMarshalledJSON(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"AccessToken": map[string]string{"safe-sentinel": "opaque<>&value"},
	})
	if err != nil {
		t.Fatal(err)
	}

	env := envelope{SchemaVersion: "cb365.msal-cache/v2", Cache: json.RawMessage(raw)}
	encoded, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Cache json.RawMessage `json:"cache"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Cache, raw) {
		t.Fatalf("embedded cache changed: got %q, want %q", decoded.Cache, raw)
	}
}

func TestRecordedV2SizeDerivation(t *testing.T) {
	const (
		recordedRawBytes    = 10194
		recordedV1Bytes     = 14043
		recordedBase64Chars = 13592
		wantV2Bytes         = 10643
	)

	// A JSON string with exactly the recorded byte length is sufficient to
	// prove the representation delta; all envelope metadata is unchanged.
	raw := json.RawMessage(`"` + strings.Repeat("a", recordedRawBytes-2) + `"`)
	if !json.Valid(raw) || len(raw) != recordedRawBytes {
		t.Fatal("test fixture must be valid JSON of the recorded raw size")
	}

	type legacyEnvelope struct {
		Cache string `json:"cache"`
	}
	legacy, err := json.Marshal(legacyEnvelope{Cache: base64.StdEncoding.EncodeToString(make([]byte, recordedRawBytes))})
	if err != nil {
		t.Fatal(err)
	}
	current, err := json.Marshal(struct {
		Cache json.RawMessage `json:"cache"`
	}{Cache: raw})
	if err != nil {
		t.Fatal(err)
	}

	delta := len(legacy) - len(current)
	if delta != recordedBase64Chars+2-recordedRawBytes {
		t.Fatalf("representation delta = %d, want %d", delta, recordedBase64Chars+2-recordedRawBytes)
	}
	if got := recordedV1Bytes - delta; got != wantV2Bytes {
		t.Fatalf("derived v2 size = %d, want %d", got, wantV2Bytes)
	}
}
