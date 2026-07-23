package main

import (
	"fmt"
	"os"
)

// commitTempFile publishes a same-directory temporary file. Without force it
// uses an exclusive hard link so a destination created after the initial check
// cannot be overwritten. With force, replacement is explicit.
func commitTempFile(tmpPath, destination string, force bool) error {
	if !force {
		if err := os.Link(tmpPath, destination); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("output file %q already exists — use --force to overwrite", destination)
			}
			return fmt.Errorf("publishing output file: %w", err)
		}
		if err := os.Remove(tmpPath); err != nil {
			return fmt.Errorf("removing temporary file after publish: %w", err)
		}
		return nil
	}

	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing existing output file: %w", err)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return fmt.Errorf("replacing output file: %w", err)
	}
	return nil
}
