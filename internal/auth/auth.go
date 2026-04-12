package auth

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/graph"
)

// graphScope converts short scope names to full Graph URIs
func graphScope(s string) string {
	if strings.HasPrefix(s, "https://") {
		return s
	}
	return "https://graph.microsoft.com/" + s
}

// GraphScopes converts a list of short scope names to full URIs
func GraphScopes(scopes []string) []string {
	full := make([]string, len(scopes))
	for i, s := range scopes {
		full[i] = graphScope(s)
	}
	return full
}

// ShouldUseIPv4 returns true if IPv4-only transport should be used.
// Checks CB365_IPV4_ONLY env var and config setting.
func ShouldUseIPv4(cfg *config.Config) bool {
	if os.Getenv("CB365_IPV4_ONLY") == "1" {
		return true
	}
	if cfg != nil && cfg.Settings.IPv4Only {
		return true
	}
	return false
}

// LoginDelegated performs device-code flow authentication
func LoginDelegated(ctx context.Context, profile *config.Profile, ipv4Only bool) (azcore.AccessToken, error) {
	opts := &azidentity.DeviceCodeCredentialOptions{
		TenantID: profile.TenantID,
		ClientID: profile.ClientID,
		UserPrompt: func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
			fmt.Println()
			fmt.Println(msg.Message)
			fmt.Println()
			return nil
		},
	}

	// Wire in IPv4-only transport if needed (some cloud regions have broken IPv6)
	if ipv4Only {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: graph.NewIPv4HTTPClient(),
		}
	}

	cred, err := azidentity.NewDeviceCodeCredential(opts)
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("creating device code credential: %w", err)
	}

	scopes := GraphScopes(profile.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scopes,
	})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("acquiring token: %w", err)
	}

	return token, nil
}

// TokenInfo represents decoded JWT claims for display
// SECURITY: This is for display only — never contains the raw token
type TokenInfo struct {
	Subject   string   `json:"subject,omitempty"`
	UPN       string   `json:"upn,omitempty"`
	Name      string   `json:"name,omitempty"`
	TenantID  string   `json:"tenant_id,omitempty"`
	AppName   string   `json:"app_name,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	ValidFor  string   `json:"valid_for,omitempty"`
	IsExpired bool     `json:"is_expired"`
}

// DecodeTokenInfo extracts display-safe info from a JWT access token
// SECURITY: Only extracts claims — does NOT validate the token signature
func DecodeTokenInfo(accessToken string) (*TokenInfo, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decoding JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("parsing JWT claims: %w", err)
	}

	info := &TokenInfo{}

	if v, ok := claims["sub"].(string); ok {
		info.Subject = v
	}
	if v, ok := claims["upn"].(string); ok {
		info.UPN = v
	}
	if v, ok := claims["name"].(string); ok {
		info.Name = v
	}
	if v, ok := claims["tid"].(string); ok {
		info.TenantID = v
	}
	if v, ok := claims["app_displayname"].(string); ok {
		info.AppName = v
	}
	if v, ok := claims["scp"].(string); ok {
		info.Scopes = strings.Split(v, " ")
	}

	if exp, ok := claims["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		info.ExpiresAt = expTime.Format(time.RFC3339)
		info.IsExpired = time.Now().After(expTime)
		if !info.IsExpired {
			info.ValidFor = time.Until(expTime).Round(time.Second).String()
		}
	}

	return info, nil
}


// LoginAppOnly performs client credentials flow authentication (app-only).
// The client secret is stored encrypted for unattended token refresh.
func LoginAppOnly(ctx context.Context, profile *config.Profile, clientSecret string, ipv4Only bool) (azcore.AccessToken, error) {
	opts := &azidentity.ClientSecretCredentialOptions{}

	if ipv4Only {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: graph.NewIPv4HTTPClient(),
		}
	}

	cred, err := azidentity.NewClientSecretCredential(
		profile.TenantID, profile.ClientID, clientSecret, opts,
	)
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("creating client secret credential: %w", err)
	}

	scopes := GraphScopes(profile.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scopes,
	})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("acquiring app-only token: %w", err)
	}

	return token, nil
}

// RefreshAppOnly uses a stored client secret to get a fresh app-only token.
// Returns the new token and updates the cache in place. Caller must persist the cache.
func RefreshAppOnly(ctx context.Context, profile *config.Profile, cache *TokenCache, ipv4Only bool) (azcore.AccessToken, error) {
	if cache.ClientSecret == "" {
		return azcore.AccessToken{}, fmt.Errorf("no client secret stored for profile %q — run 'cb365 auth login --mode app-only' to re-authenticate", profile.Name)
	}

	return LoginAppOnly(ctx, profile, cache.ClientSecret, ipv4Only)
}

// LoginCertificate performs certificate-based authentication (app-only).
// The PEM file must contain both the certificate and private key.
func LoginCertificate(ctx context.Context, profile *config.Profile, pemPath string, ipv4Only bool) (azcore.AccessToken, error) {
	pemData, err := os.ReadFile(pemPath) // #nosec G304 — path from --certificate CLI flag, not untrusted input
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("reading certificate file: %w", err)
	}

	var certs []*x509.Certificate
	var privKey interface{}
	remaining := pemData

	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			cert, parseErr := x509.ParseCertificate(block.Bytes)
			if parseErr != nil {
				return azcore.AccessToken{}, fmt.Errorf("parsing certificate: %w", parseErr)
			}
			certs = append(certs, cert)
		case "RSA PRIVATE KEY":
			key, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes)
			if parseErr != nil {
				return azcore.AccessToken{}, fmt.Errorf("parsing RSA private key: %w", parseErr)
			}
			privKey = key
		case "PRIVATE KEY":
			key, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
			if parseErr != nil {
				return azcore.AccessToken{}, fmt.Errorf("parsing PKCS8 private key: %w", parseErr)
			}
			privKey = key
		case "EC PRIVATE KEY":
			key, parseErr := x509.ParseECPrivateKey(block.Bytes)
			if parseErr != nil {
				return azcore.AccessToken{}, fmt.Errorf("parsing EC private key: %w", parseErr)
			}
			privKey = key
		}
		remaining = rest
	}

	if len(certs) == 0 {
		return azcore.AccessToken{}, fmt.Errorf("no certificates found in PEM file")
	}
	if privKey == nil {
		return azcore.AccessToken{}, fmt.Errorf("no private key found in PEM file")
	}

	opts := &azidentity.ClientCertificateCredentialOptions{}
	if ipv4Only {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: graph.NewIPv4HTTPClient(),
		}
	}

	cred, err := azidentity.NewClientCertificateCredential(
		profile.TenantID, profile.ClientID, certs, privKey, opts,
	)
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("creating certificate credential: %w", err)
	}

	scopes := GraphScopes(profile.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"https://graph.microsoft.com/.default"}
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scopes,
	})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("acquiring certificate token: %w", err)
	}

	return token, nil
}

// RefreshCertificate uses a stored certificate path to get a fresh app-only token.
// Returns the new token. Caller must persist the cache.
func RefreshCertificate(ctx context.Context, profile *config.Profile, cache *TokenCache, ipv4Only bool) (azcore.AccessToken, error) {
	if cache.CertPath == "" {
		return azcore.AccessToken{}, fmt.Errorf("no certificate path stored for profile %q — run 'cb365 auth login --mode app-only --certificate <path>'", profile.Name)
	}

	return LoginCertificate(ctx, profile, cache.CertPath, ipv4Only)
}
