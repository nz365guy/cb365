//go:build !cgo

package bwsadapter

import "errors"

// ErrUnavailable is the compile-time stub error required by ADR-0057 for
// unsupported or CGO_ENABLED=0 builds. Such builds must fail closed — they
// must not restore any local delegated cache.
var ErrUnavailable = errors.New("managed delegated authentication unavailable on this build")

// Available reports whether the managed delegated provider can exist in this build.
func Available() bool { return false }

// Adapter is never constructible in a no-cgo build.
type Adapter struct{}

// New always refuses on a no-cgo build.
func New(_, _ string) (*Adapter, error) { return nil, ErrUnavailable }

// Close is a no-op on the stub.
func (a *Adapter) Close() {}
