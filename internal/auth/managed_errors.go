package auth

import (
	"errors"
	"fmt"
)

// ManagedErrorClass is a stable, secret-free error category for managed
// delegated authentication. Callers may render the class and operation, but
// never an SDK or identity-provider response body.
type ManagedErrorClass string

const (
	ManagedCacheUnavailable  ManagedErrorClass = "managed_cache_unavailable"
	ManagedCacheInvalid      ManagedErrorClass = "managed_cache_invalid"
	ReauthenticationRequired ManagedErrorClass = "reauthentication_required"
	ManagedCacheConflict     ManagedErrorClass = "managed_cache_conflict"
)

// ManagedError intentionally does not implement Unwrap. The cause is retained
// for in-process classification only so provider responses cannot leak through
// generic error formatting.
type ManagedError struct {
	Class     ManagedErrorClass
	Operation string
	cause     error
}

func (e *ManagedError) Error() string {
	if e.Operation == "" {
		return string(e.Class)
	}
	return fmt.Sprintf("%s: %s failed", e.Class, e.Operation)
}

func managedError(class ManagedErrorClass, operation string, cause error) error {
	return &ManagedError{Class: class, Operation: operation, cause: cause}
}

// ManagedErrorClassOf returns a stable class without exposing the underlying
// provider failure.
func ManagedErrorClassOf(err error) (ManagedErrorClass, bool) {
	var managed *ManagedError
	if !errors.As(err, &managed) {
		return "", false
	}
	return managed.Class, true
}
