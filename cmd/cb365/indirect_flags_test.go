package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExpandIndirectStringFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	valuePath := filepath.Join(dir, "value.txt")
	listPath := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(valuePath, []byte("private subject\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(listPath, []byte("one@example.com\ntwo@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "test"}
	var subject string
	var attendees []string
	cmd.Flags().StringVar(&subject, "subject", "", "")
	cmd.Flags().StringArrayVar(&attendees, "attendee", nil, "")
	if err := cmd.Flags().Set("subject", "@"+valuePath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("attendee", "@"+listPath); err != nil {
		t.Fatal(err)
	}

	if err := expandIndirectStringFlags(cmd); err != nil {
		t.Fatal(err)
	}
	if subject != "private subject" {
		t.Fatalf("subject = %q", subject)
	}
	wantAttendees := []string{"one@example.com", "two@example.com"}
	if !reflect.DeepEqual(attendees, wantAttendees) {
		t.Fatalf("attendees = %#v, want %#v", attendees, wantAttendees)
	}
}

func TestExpandIndirectStringFlagsStdinAndLiteralAt(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.SetIn(bytes.NewBufferString("from stdin\n"))
	var body string
	var literal string
	cmd.Flags().StringVar(&body, "body", "", "")
	cmd.Flags().StringVar(&literal, "literal", "", "")
	if err := cmd.Flags().Set("body", "@-"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("literal", "@@mention"); err != nil {
		t.Fatal(err)
	}

	if err := expandIndirectStringFlags(cmd); err != nil {
		t.Fatal(err)
	}
	if body != "from stdin" {
		t.Fatalf("body = %q", body)
	}
	if literal != "@mention" {
		t.Fatalf("literal = %q", literal)
	}
}

func TestExpandIndirectStringFlagsRejectsOversize(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.SetIn(bytes.NewReader(bytes.Repeat([]byte("x"), maxIndirectFlagBytes+1)))
	var body string
	cmd.Flags().StringVar(&body, "body", "", "")
	if err := cmd.Flags().Set("body", "@-"); err != nil {
		t.Fatal(err)
	}
	if err := expandIndirectStringFlags(cmd); err == nil {
		t.Fatal("expected oversized indirect input to fail")
	}
}

func TestRootCommandExpandsIndirectStringFlags(t *testing.T) {
	dir := t.TempDir()
	valuePath := filepath.Join(dir, "subject.txt")
	if err := os.WriteFile(valuePath, []byte("Private subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dir, "missing.txt")

	subjectFlag := mailSendCmd.Flags().Lookup("subject")
	originalSubject := subjectFlag.Value.String()
	originalChanged := subjectFlag.Changed
	originalRunE := mailSendCmd.RunE
	t.Cleanup(func() {
		mailSendCmd.RunE = originalRunE
		if err := subjectFlag.Value.Set(originalSubject); err != nil {
			t.Errorf("restoring --subject: %v", err)
		}
		subjectFlag.Changed = originalChanged
		rootCmd.SetArgs(nil)
	})

	var receivedSubject string
	mailSendCmd.RunE = func(cmd *cobra.Command, args []string) error {
		receivedSubject = mailSendSubject
		return nil
	}

	tests := []struct {
		name         string
		value        string
		want         string
		wantErrParts []string
	}{
		{
			name:  "file",
			value: "@" + valuePath,
			want:  "Private subject",
		},
		{
			name:  "literal at",
			value: "@@literal",
			want:  "@literal",
		},
		{
			name:         "missing file",
			value:        "@" + missingPath,
			wantErrParts: []string{"--subject", missingPath},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := subjectFlag.Value.Set(subjectFlag.DefValue); err != nil {
				t.Fatal(err)
			}
			subjectFlag.Changed = false
			receivedSubject = ""
			rootCmd.SetArgs([]string{"mail", "send", "--subject", tt.value})

			err := rootCmd.Execute()
			if len(tt.wantErrParts) == 0 {
				if err != nil {
					t.Fatal(err)
				}
				if receivedSubject != tt.want {
					t.Fatalf("subject = %q, want %q", receivedSubject, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatal("expected indirect input to fail")
			}
			for _, part := range tt.wantErrParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not contain %q", err, part)
				}
			}
		})
	}
}

func TestRootPersistentPreRunLeavesUnsetDefaultsUntouched(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var subject string
	cmd.Flags().StringVar(&subject, "subject", "@default-value", "")

	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if subject != "@default-value" {
		t.Fatalf("subject = %q, want unchanged default", subject)
	}
}
