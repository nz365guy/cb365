package main

import (
	"os"
	"testing"
	"time"
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
		name                               string
		newStart, newEnd, exStart, exEnd    time.Time
		want                               bool
	}{
		{"exact overlap", base, base.Add(time.Hour), base, base.Add(time.Hour), true},
		{"partial overlap start", base, base.Add(time.Hour), base.Add(30*time.Minute), base.Add(90*time.Minute), true},
		{"no overlap before", base, base.Add(time.Hour), base.Add(2*time.Hour), base.Add(3*time.Hour), false},
		{"no overlap after", base.Add(2*time.Hour), base.Add(3*time.Hour), base, base.Add(time.Hour), false},
		{"adjacent no overlap", base, base.Add(time.Hour), base.Add(time.Hour), base.Add(2*time.Hour), false},
	}
	for _, tt := range tests {
		got := hasTimeOverlap(tt.newStart, tt.newEnd, tt.exStart, tt.exEnd)
		if got != tt.want {
			t.Errorf("%s: hasTimeOverlap = %v, want %v", tt.name, got, tt.want)
		}
	}
}

