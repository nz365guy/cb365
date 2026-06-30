//go:build darwin

package auth

// On macOS the azidentity/cache persistent backend requires cgo (it uses the
// system keychain), and cb365 release builds set CGO_ENABLED=0. There is no
// init here, so msalCache stays zero-value and MSAL uses an in-memory cache.
// Tokens are not persisted between runs on macOS.
