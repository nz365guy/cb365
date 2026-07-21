//go:build cgo

// Package bwsadapter is the ADR-0057 spike stub: it proves the official
// Bitwarden Go SDK v2 (cgo, statically linked libbitwarden_c) compiles and
// links on the target toolchain and that an SDK-backed type can satisfy the
// MSAL cache.ExportReplace contract. It intentionally implements no secret
// I/O; the real provider is the separately authorised item cb365 #43.
package bwsadapter

import (
	"context"
	"errors"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	sdk "github.com/bitwarden/sdk-go/v2"
)

// Available reports whether the managed delegated provider can exist in this build.
func Available() bool { return true }

// Adapter holds a live Bitwarden SDK client handle and exposes the MSAL
// external-cache surface the production provider will implement.
type Adapter struct {
	client sdk.BitwardenClientInterface
}

var _ cache.ExportReplace = (*Adapter)(nil)

// New constructs the SDK client against explicit EU endpoints. Construction
// performs FFI initialisation only — no network call, no authentication, no
// secret access.
func New(apiURL, identityURL string) (*Adapter, error) {
	client, err := sdk.NewBitwardenClient(&apiURL, &identityURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: client}, nil
}

// Replace is a stub: the BWS read path is implemented in cb365 #43.
func (a *Adapter) Replace(context.Context, cache.Unmarshaler, cache.ReplaceHints) error {
	return errors.New("spike stub: BWS read path is not implemented (cb365 #43)")
}

// Export is a stub: the BWS write path is implemented in cb365 #43.
func (a *Adapter) Export(context.Context, cache.Marshaler, cache.ExportHints) error {
	return errors.New("spike stub: BWS write path is not implemented (cb365 #43)")
}

// Close releases the SDK client.
func (a *Adapter) Close() { a.client.Close() }
