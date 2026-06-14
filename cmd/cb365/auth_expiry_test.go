package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// selfSignedCertPEM builds an in-memory self-signed certificate with the given
// NotAfter and returns its PEM encoding plus the signing key. No disk I/O.
func selfSignedCertPEM(t *testing.T, notAfter time.Time) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "cb365-test"},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return certPEM, key
}

func TestParseCertNotAfterBytes(t *testing.T) {
	want := time.Now().Add(42 * 24 * time.Hour).Truncate(time.Second)
	certPEM, _ := selfSignedCertPEM(t, want)

	got, err := parseCertNotAfterBytes(certPEM)
	if err != nil {
		t.Fatalf("parseCertNotAfterBytes: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("NotAfter mismatch: got %s, want %s", got, want)
	}
}

func TestParseCertNotAfterBytes_SkipsPrivateKeyBlock(t *testing.T) {
	want := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	certPEM, key := selfSignedCertPEM(t, want)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	// Private-key block first, then the certificate: the helper must skip the key.
	combined := append(append([]byte{}, keyPEM...), certPEM...)

	got, err := parseCertNotAfterBytes(combined)
	if err != nil {
		t.Fatalf("parseCertNotAfterBytes (combined): %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("NotAfter mismatch: got %s, want %s", got, want)
	}
}

func TestParseCertNotAfterBytes_NoCertificate(t *testing.T) {
	if _, err := parseCertNotAfterBytes([]byte("not a pem block")); err == nil {
		t.Fatal("expected an error when no CERTIFICATE block is present")
	}
}

func TestEnvIntDefault(t *testing.T) {
	const key = "CB365_TEST_ENVINT_XYZ"

	if got := envIntDefault(key, 30); got != 30 {
		t.Fatalf("unset: got %d, want 30", got)
	}

	t.Setenv(key, "7")
	if got := envIntDefault(key, 30); got != 7 {
		t.Fatalf("valid: got %d, want 7", got)
	}

	t.Setenv(key, "not-a-number")
	if got := envIntDefault(key, 30); got != 30 {
		t.Fatalf("invalid: got %d, want 30 (fallback)", got)
	}

	t.Setenv(key, "-5")
	if got := envIntDefault(key, 30); got != 30 {
		t.Fatalf("negative: got %d, want 30 (fallback)", got)
	}
}
