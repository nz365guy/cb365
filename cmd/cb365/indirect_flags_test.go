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

func TestRootCmdExpandsIndirectStringFlags(t *testing.T) {
	dir := t.TempDir()
	valuePath := filepath.Join(dir, "subject.txt")
	if err := os.WriteFile(valuePath, []byte("Private subject\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var subject string
	var literal string
	var untouchedDefault string
	var ran bool
	testCmd := &cobra.Command{
		Use: "test-indirect-flags-success",
		Run: func(cmd *cobra.Command, args []string) {
			ran = true
		},
	}
	testCmd.Flags().StringVar(&subject, "subject", "", "")
	testCmd.Flags().StringVar(&literal, "literal", "", "")
	testCmd.Flags().StringVar(&untouchedDefault, "untouched", "@default", "")
	rootCmd.AddCommand(testCmd)
	t.Cleanup(func() {
		rootCmd.RemoveCommand(testCmd)
		rootCmd.SetArgs(nil)
	})

	rootCmd.SetArgs([]string{
		testCmd.Name(),
		"--subject", "@" + valuePath,
		"--literal", "@@literal",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("command did not run")
	}
	if subject != "Private subject" {
		t.Fatalf("subject = %q, want %q", subject, "Private subject")
	}
	if literal != "@literal" {
		t.Fatalf("literal = %q, want %q", literal, "@literal")
	}
	if untouchedDefault != "@default" {
		t.Fatalf("untouched default = %q, want %q", untouchedDefault, "@default")
	}
}

func TestRootCmdReportsIndirectStringFlagPathError(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.txt")
	var subject string
	var ran bool
	testCmd := &cobra.Command{
		Use: "test-indirect-flags-missing-path",
		Run: func(cmd *cobra.Command, args []string) {
			ran = true
		},
	}
	testCmd.Flags().StringVar(&subject, "subject", "", "")
	rootCmd.AddCommand(testCmd)
	t.Cleanup(func() {
		rootCmd.RemoveCommand(testCmd)
		rootCmd.SetArgs(nil)
	})

	rootCmd.SetArgs([]string{testCmd.Name(), "--subject", "@" + missingPath})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected missing indirect input to fail")
	}
	if ran {
		t.Fatal("command ran after indirect input expansion failed")
	}
	if !strings.Contains(err.Error(), "--subject") || !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error = %q, want flag name and input path", err)
	}
}

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
