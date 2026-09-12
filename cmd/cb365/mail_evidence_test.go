package main

import (
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"slices"
	"testing"
)

func TestMailGetRequestsEvidenceWithoutChangingDefaultIdentifierType(t *testing.T) {
	config := newMailGetRequestConfiguration(false)
	for _, field := range []string{"body", "ccRecipients", "conversationId", "webLink", "internetMessageHeaders", "internetMessageId"} {
		if !slices.Contains(config.QueryParameters.Select, field) {
			t.Fatalf("missing selected field %s", field)
		}
	}
	if config.Headers != nil {
		t.Fatal("default read must not silently change identifier type")
	}
	immutable := newMailGetRequestConfiguration(true)
	values := immutable.Headers.Get("Prefer")
	if len(values) != 1 || values[0] != `IdType="ImmutableId"` {
		t.Fatal("immutable identifier preference absent")
	}
}

func TestMessageJSONPreservesSelectedEvidenceHeaders(t *testing.T) {
	message := models.NewMessage()
	identifier := "<fixture@example.test>"
	message.SetInternetMessageId(&identifier)
	headers := []models.InternetMessageHeaderable{}
	for _, pair := range [][2]string{{"Authentication-Results", "mx.example.test; dmarc=pass header.from=example.test"}, {"Authentication-Results", "untrusted-duplicate"}, {"Message-ID", identifier}, {"X-Unselected", "must-not-be-exposed"}} {
		header := models.NewInternetMessageHeader()
		name, value := pair[0], pair[1]
		header.SetName(&name)
		header.SetValue(&value)
		headers = append(headers, header)
	}
	message.SetInternetMessageHeaders(headers)
	result := formatMessageJSON(message)
	if result["internet_message_id"] != identifier {
		t.Fatal("stable message ID missing")
	}
	auth := result["authentication_headers"].(map[string]string)
	if auth["authentication-results"] != "mx.example.test; dmarc=pass header.from=example.test" {
		t.Fatal("first authentication result was replaced")
	}
	metadata := result["metadata_headers"].(map[string]string)
	if metadata["message-id"] != identifier || len(metadata) != 1 {
		t.Fatal("metadata whitelist failed")
	}
}

func TestTranslateIDValidatesBeforeAuthentication(t *testing.T) {
	for _, values := range [][3]string{{"", "restId", "restImmutableEntryId"}, {"bad\nvalue", "restId", "restImmutableEntryId"}, {"opaque-id", "unknown", "restId"}, {"opaque-id", "restId", "restId"}} {
		if _, err := newTranslateIDRequest(values[0], values[1], values[2]); err == nil {
			t.Fatal("invalid translation accepted")
		}
	}
	oldID, oldSource, oldTarget := mailTranslateID, mailTranslateSource, mailTranslateTarget
	defer func() { mailTranslateID, mailTranslateSource, mailTranslateTarget = oldID, oldSource, oldTarget }()
	mailTranslateID = ""
	mailTranslateSource = "restId"
	mailTranslateTarget = "restImmutableEntryId"
	if err := mailTranslateIDCmd.RunE(mailTranslateIDCmd, nil); err == nil || err.Error() != "--id must be one valid Exchange identifier" {
		t.Fatal("invalid ID did not stop before authentication")
	}
}

func TestTranslateIDRequestAndResultAreExactlyBound(t *testing.T) {
	request, err := newTranslateIDRequest("opaque-source", "restId", "restImmutableEntryId")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(request.GetInputIds(), []string{"opaque-source"}) || *request.GetSourceIdType() != models.RESTID_EXCHANGEIDFORMAT || *request.GetTargetIdType() != models.RESTIMMUTABLEENTRYID_EXCHANGEIDFORMAT {
		t.Fatal("request binding changed")
	}
	value := models.NewConvertIdResult()
	source, target := "opaque-source", "opaque-target"
	value.SetSourceId(&source)
	value.SetTargetId(&target)
	result, err := translatedMessageID(source, []models.ConvertIdResultable{value})
	if err != nil || result != target {
		t.Fatal("valid translation rejected")
	}
	for _, values := range [][]models.ConvertIdResultable{nil, {nil}, {value, value}} {
		if _, err := translatedMessageID(source, values); err == nil {
			t.Fatal("ambiguous translation accepted")
		}
	}
	if _, err := translatedMessageID("different-source", []models.ConvertIdResultable{value}); err == nil {
		t.Fatal("wrong source accepted")
	}
}
