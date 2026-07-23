//go:build !windows

package auth

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
