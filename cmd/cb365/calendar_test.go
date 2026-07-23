package main

import (
	"os"
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// ─── Unit tests (safety rules, no Graph API needed) ───

func TestParseRFC3339StrictValid(t *testing.T) {
	tests := []string{
		"2026-04-10T09:00:00+12:00",
		"2026-04-10T09:00:00Z",
		"2026-04-10T09:00:00-05:00",
		"2026-12-25T00:00:00+13:00",
	}
	for _, s := range tests {
		_, err := parseRFC3339Strict(s)
		if err != nil {
			t.Errorf("parseRFC3339Strict(%q) unexpected error: %v", s, err)
		}
	}
}

func TestParseRFC3339StrictRejectsBare(t *testing.T) {
	bare := []string{
		"2026-04-10T09:00:00",
		"2026-04-10T09:00:00.000",
	}
	for _, s := range bare {
		_, err := parseRFC3339Strict(s)
		if err == nil {
			t.Errorf("parseRFC3339Strict(%q) should reject bare datetime", s)
		}
	}
}

func TestGraphDateTimePartsUsesSupportedUTCZone(t *testing.T) {
	tests := []struct {
		name         string
		input        time.Time
		wantDateTime string
	}{
		{
			name:         "numeric positive offset",
			input:        time.Date(2026, 4, 10, 9, 0, 0, 0, time.FixedZone("Local", 12*60*60)),
			wantDateTime: "2026-04-09T21:00:00",
		},
		{
			name:         "numeric negative offset",
			input:        time.Date(2026, 4, 10, 9, 0, 0, 0, time.FixedZone("", -5*60*60)),
			wantDateTime: "2026-04-10T14:00:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDateTime, gotTimeZone := graphDateTimeParts(tt.input)
			if gotDateTime != tt.wantDateTime {
				t.Fatalf("graphDateTimeParts() datetime = %q, want %q", gotDateTime, tt.wantDateTime)
			}
			if gotTimeZone != "UTC" {
				t.Fatalf("graphDateTimeParts() timezone = %q, want UTC", gotTimeZone)
			}
		})
	}
}

func TestFormatEventJSONIncludesCategories(t *testing.T) {
	event := models.NewEvent()
	event.SetCategories([]string{"Agent Meeting", "Customer"})

	item := formatEventJSON(event)
	categories, ok := item["categories"].([]string)
	if !ok {
		t.Fatalf("formatEventJSON() categories type = %T, want []string", item["categories"])
	}
	if len(categories) != 2 || categories[0] != "Agent Meeting" || categories[1] != "Customer" {
		t.Fatalf("formatEventJSON() categories = %#v", categories)
	}
}

func TestRejectPastEvent(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	err := rejectPastEvent(past, "create")
	if err == nil {
		t.Error("rejectPastEvent should reject past time")
	}

	future := time.Now().Add(24 * time.Hour)
	err = rejectPastEvent(future, "create")
	if err != nil {
		t.Errorf("rejectPastEvent should accept future time: %v", err)
	}
}

func TestLocalNowUsesTimezone(t *testing.T) {
	os.Setenv("CB365_TIMEZONE", "UTC")
	defer os.Unsetenv("CB365_TIMEZONE")

	now := localNow()
	if now.Location().String() != "UTC" {
		t.Errorf("localNow() with CB365_TIMEZONE=UTC: got location %q", now.Location().String())
	}
}

func TestLocalNowDefaultsToSystem(t *testing.T) {
	os.Unsetenv("CB365_TIMEZONE")

	now := localNow()
	// Should not panic and should return a valid time
	if now.IsZero() {
		t.Error("localNow() returned zero time")
	}
}

func TestHasTimeOverlap(t *testing.T) {
	base := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name                             string
		newStart, newEnd, exStart, exEnd time.Time
		want                             bool
	}{
		{"exact overlap", base, base.Add(time.Hour), base, base.Add(time.Hour), true},
		{"partial overlap start", base, base.Add(time.Hour), base.Add(30 * time.Minute), base.Add(90 * time.Minute), true},
		{"no overlap before", base, base.Add(time.Hour), base.Add(2 * time.Hour), base.Add(3 * time.Hour), false},
		{"no overlap after", base.Add(2 * time.Hour), base.Add(3 * time.Hour), base, base.Add(time.Hour), false},
		{"adjacent no overlap", base, base.Add(time.Hour), base.Add(time.Hour), base.Add(2 * time.Hour), false},
	}
	for _, tt := range tests {
		got := hasTimeOverlap(tt.newStart, tt.newEnd, tt.exStart, tt.exEnd)
		if got != tt.want {
			t.Errorf("%s: hasTimeOverlap = %v, want %v", tt.name, got, tt.want)
		}
	}
}
