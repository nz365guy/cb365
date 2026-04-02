package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Format represents the output format
type Format int

const (
	FormatHuman Format = iota
	FormatJSON
	FormatPlain
)

// Resolve determines the output format from flags
func Resolve(jsonFlag, plainFlag bool) Format {
	if jsonFlag {
		return FormatJSON
	}
	if plainFlag {
		return FormatPlain
	}
	return FormatHuman
}

// JSON writes a value as indented JSON to stdout
func JSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Plain writes tab-separated values to stdout
func Plain(rows [][]string) {
	for _, row := range rows {
		fmt.Println(strings.Join(row, "\t"))
	}
}

// Table writes a formatted table to stdout
func Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	fmt.Fprintln(w, strings.Repeat("─", len(strings.Join(headers, "  "))))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// Success prints a success message to stderr (so stdout stays clean for --json)
func Success(msg string) {
	fmt.Fprintf(os.Stderr, "✓ %s\n", msg)
}

// Error prints an error message to stderr
func Error(msg string) {
	fmt.Fprintf(os.Stderr, "✗ %s\n", msg)
}

// Info prints an info message to stderr
func Info(msg string) {
	fmt.Fprintf(os.Stderr, "→ %s\n", msg)
}
