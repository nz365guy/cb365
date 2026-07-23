package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const maxIndirectFlagBytes = 1024 * 1024

// expandIndirectStringFlags lets callers keep business content out of argv.
// A scalar string value of @path is read from that file; @- reads stdin. Use
// @@value to pass a literal value beginning with @. Slice values support the
// same syntax and interpret an indirect file as one non-empty value per line.
func expandIndirectStringFlags(cmd *cobra.Command) error {
	seen := make(map[*pflag.Flag]struct{})
	sets := []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()}
	for _, set := range sets {
		var visitErr error
		set.Visit(func(flag *pflag.Flag) {
			if visitErr != nil {
				return
			}
			if _, ok := seen[flag]; ok {
				return
			}
			seen[flag] = struct{}{}

			if slice, ok := flag.Value.(pflag.SliceValue); ok {
				values := slice.GetSlice()
				expanded := make([]string, 0, len(values))
				for _, value := range values {
					items, err := indirectSliceValue(cmd, value)
					if err != nil {
						visitErr = fmt.Errorf("--%s: %w", flag.Name, err)
						return
					}
					expanded = append(expanded, items...)
				}
				if err := slice.Replace(expanded); err != nil {
					visitErr = fmt.Errorf("--%s: applying indirect value: %w", flag.Name, err)
				}
				return
			}

			if flag.Value.Type() != "string" {
				return
			}
			value, changed, err := indirectScalarValue(cmd, flag.Value.String())
			if err != nil {
				visitErr = fmt.Errorf("--%s: %w", flag.Name, err)
				return
			}
			if changed {
				if err := flag.Value.Set(value); err != nil {
					visitErr = fmt.Errorf("--%s: applying indirect value: %w", flag.Name, err)
				}
			}
		})
		if visitErr != nil {
			return visitErr
		}
	}
	return nil
}

func indirectScalarValue(cmd *cobra.Command, value string) (string, bool, error) {
	if strings.HasPrefix(value, "@@") {
		return value[1:], true, nil
	}
	if !strings.HasPrefix(value, "@") || len(value) == 1 {
		return value, false, nil
	}
	content, err := readIndirectFlag(cmd, value[1:])
	if err != nil {
		return "", false, err
	}
	return trimOneLineEnding(content), true, nil
}

func indirectSliceValue(cmd *cobra.Command, value string) ([]string, error) {
	if strings.HasPrefix(value, "@@") {
		return []string{value[1:]}, nil
	}
	if !strings.HasPrefix(value, "@") || len(value) == 1 {
		return []string{value}, nil
	}
	content, err := readIndirectFlag(cmd, value[1:])
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func readIndirectFlag(cmd *cobra.Command, source string) (string, error) {
	var reader io.Reader
	var closeFn func() error
	if source == "-" {
		reader = cmd.InOrStdin()
	} else {
		file, err := os.Open(source)
		if err != nil {
			return "", fmt.Errorf("opening indirect input %q: %w", source, err)
		}
		reader = file
		closeFn = file.Close
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxIndirectFlagBytes+1))
	if closeFn != nil {
		if closeErr := closeFn(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return "", fmt.Errorf("reading indirect input %q: %w", source, err)
	}
	if len(data) > maxIndirectFlagBytes {
		return "", fmt.Errorf("indirect input exceeds %d bytes", maxIndirectFlagBytes)
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return "", fmt.Errorf("indirect input contains a NUL byte")
	}
	return string(data), nil
}

func trimOneLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	return strings.TrimSuffix(value, "\n")
}
