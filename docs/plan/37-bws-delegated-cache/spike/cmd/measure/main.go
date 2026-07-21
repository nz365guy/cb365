// measure is the Gate 1 evidence tool for cb365 #42 (ADR-0057).
//
// It performs one operator-assisted device-code login against the approved
// test-tenant profile using an MSAL Go public client with an in-memory
// cache.ExportReplace capture — the same extension point the production
// provider will use — then prints ONLY evidence-safe numbers:
//
//	raw exported cache size, its SHA-256 digest, the base64 length, and the
//	size of the full ADR-0057 envelope built from real binding values.
//
// The cache bytes never touch disk, argv, or the output. No BWS record is
// read or written. A silent acquisition is performed after login so the
// captured cache reflects the steady-state refresh path.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

// captureCache is an in-memory ExportReplace that records the largest
// exported cache. It never persists or prints cache bytes.
type captureCache struct {
	data    []byte
	exports int
}

func (c *captureCache) Replace(_ context.Context, u cache.Unmarshaler, _ cache.ReplaceHints) error {
	if len(c.data) > 0 {
		return u.Unmarshal(c.data)
	}
	return nil
}

func (c *captureCache) Export(_ context.Context, m cache.Marshaler, _ cache.ExportHints) error {
	b, err := m.Marshal()
	if err != nil {
		return err
	}
	c.exports++
	if len(b) > len(c.data) {
		c.data = append(c.data[:0], b...)
	}
	return nil
}

// envelope mirrors the ADR-0057 record contract exactly, so the measured
// size is the size that must fit the BWS secret-value limit.
type envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Binding       struct {
		TenantID      string `json:"tenantId"`
		ClientID      string `json:"clientId"`
		HomeAccountID string `json:"homeAccountId"`
		Profile       string `json:"profile"`
	} `json:"binding"`
	Generation     int     `json:"generation"`
	PreviousDigest *string `json:"previousDigest"`
	Cache          string  `json:"cache"`
	UpdatedAt      string  `json:"updatedAt"`
	Writer         string  `json:"writer"`
}

func main() {
	tenantID := flag.String("tenant", "", "test-tenant ID (UUID) of the approved test profile")
	clientID := flag.String("client", "", "client ID (UUID) of the approved test profile")
	scopesFlag := flag.String("scopes", "User.Read", "comma-separated Graph scopes (short names ok)")
	profile := flag.String("profile", "test-spike42", "profile name used in the envelope binding")
	flag.Parse()

	if *tenantID == "" || *clientID == "" {
		fmt.Fprintln(os.Stderr, "usage: measure -tenant <uuid> -client <uuid> [-scopes User.Read,...] [-profile name]")
		os.Exit(2)
	}

	var scopes []string
	for _, s := range strings.Split(*scopesFlag, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, "https://") {
			s = "https://graph.microsoft.com/" + s
		}
		scopes = append(scopes, s)
	}

	cap := &captureCache{}
	client, err := public.New(*clientID,
		public.WithAuthority("https://login.microsoftonline.com/"+*tenantID),
		public.WithCache(cap),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating MSAL client: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	dc, err := client.AcquireTokenByDeviceCode(ctx, scopes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting device-code flow: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println(dc.Result.Message)
	fmt.Println()

	result, err := dc.AuthenticationResult(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "completing device-code flow: %v\n", err)
		os.Exit(1)
	}

	// Silent acquisition mirrors the steady-state refresh path (Replace →
	// acquire → Export) so the captured cache is representative.
	if _, err := client.AcquireTokenSilent(ctx, scopes, public.WithSilentAccount(result.Account)); err != nil {
		fmt.Fprintf(os.Stderr, "silent acquisition after login failed: %v\n", err)
		os.Exit(1)
	}

	if len(cap.data) == 0 {
		fmt.Fprintln(os.Stderr, "no cache was exported; cannot measure")
		os.Exit(1)
	}

	sum := sha256.Sum256(cap.data)
	b64 := base64.StdEncoding.EncodeToString(cap.data)

	prev := "sha256:" + hex.EncodeToString(sum[:]) // realistic worst-case field size
	env := envelope{SchemaVersion: "cb365.msal-cache/v1", Generation: 1000, PreviousDigest: &prev, Cache: b64,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339), Writer: hostnameOr("openclaw-vm")}
	env.Binding.TenantID = *tenantID
	env.Binding.ClientID = *clientID
	env.Binding.HomeAccountID = result.Account.HomeAccountID
	env.Binding.Profile = *profile

	envBytes, err := json.Marshal(env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshalling envelope: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("gate1_measurement (evidence-safe: sizes and digests only)")
	fmt.Printf("  scopes: %s\n", strings.Join(scopes, " "))
	fmt.Printf("  exports_observed: %d\n", cap.exports)
	fmt.Printf("  raw_cache_bytes: %d\n", len(cap.data))
	fmt.Printf("  raw_cache_sha256: %s\n", hex.EncodeToString(sum[:]))
	fmt.Printf("  base64_cache_chars: %d\n", len(b64))
	fmt.Printf("  envelope_bytes: %d\n", len(envBytes))
	fmt.Printf("  home_account_id_chars: %d\n", len(result.Account.HomeAccountID))
}

func hostnameOr(fallback string) string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return fallback
}
