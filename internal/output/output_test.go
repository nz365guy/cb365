package output

import "testing"

func TestSanitizeRowProtectsTSVAndTerminal(t *testing.T) {
	got := sanitizeRow([]string{"=WEBSERVICE(\"https://example.invalid\")", "line\nbreak", "ok\x1b[31m"}, true)
	if got[0][0] != '\'' {
		t.Fatalf("formula-like field was not neutralized: %q", got[0])
	}
	if got[1] != "line break" {
		t.Fatalf("record separator was not flattened: %q", got[1])
	}
	if got[2] != "ok[31m" {
		t.Fatalf("escape byte was not removed: %q", got[2])
	}
}
