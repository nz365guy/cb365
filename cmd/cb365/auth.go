package main

import (
	"context"
	"encoding/json"
	"strings"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/nz365guy/cb365/internal/auth"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate and manage profiles",
}

// --- auth login ---

var (
	loginTenant       string
	loginClient       string
	loginScopes       []string
	loginName         string
	loginMode         string
	loginClientSecret string
	loginCertificate  string
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Entra ID (device-code flow)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if loginTenant == "" || loginClient == "" {
			return fmt.Errorf("--tenant and --client are required")
		}

		mode := config.AuthModeDelegated
		if loginMode == "app-only" {
			mode = config.AuthModeAppOnly
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		profileName := loginName
		if profileName == "" {
			profileName = "default"
		}
		if flagProfile != "" {
			profileName = flagProfile
		}

		profile := &config.Profile{
			Name:     profileName,
			TenantID: loginTenant,
			ClientID: loginClient,
			AuthMode: mode,
			Scopes:   loginScopes,
		}

		ipv4Only := auth.ShouldUseIPv4(cfg)
		if ipv4Only && flagVerbose {
			output.Info("Using IPv4-only transport (CB365_IPV4_ONLY=1)")
		}

		// Pre-flight: verify token store is accessible BEFORE starting auth flow.
		// This prevents the user from completing browser auth only to have the
		// token discarded because the store can't be initialized.
		if _, err := auth.LoadToken("__preflight_check__"); err != nil {
			// LoadToken failing on a non-existent profile is expected (key not found).
			// But if the error is about store initialization, that's a real problem.
			errMsg := err.Error()
			if strings.Contains(errMsg, "CB365_KEYRING_PASSWORD") || strings.Contains(errMsg, "token store") || strings.Contains(errMsg, "keychain") {
				return fmt.Errorf("token store not available — fix this BEFORE authenticating:\n  %s", errMsg)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var tokenStr string
		var expiresOn time.Time
		var clientSecretToStore string
		var certPathToStore string
		var authRecordStr string

		if mode == config.AuthModeAppOnly {
			// App-only: certificate auth OR client credentials
			if loginCertificate != "" {
				// Certificate-based app-only auth (no client secret needed)
				output.Info(fmt.Sprintf("Authenticating profile %q via certificate...", profileName))

				certToken, certErr := auth.LoginCertificate(ctx, profile, loginCertificate, ipv4Only)
				if certErr != nil {
					return fmt.Errorf("authentication failed: %w", certErr)
				}
				tokenStr = certToken.Token
				expiresOn = certToken.ExpiresOn
				profile.Username = "(app-only/certificate)"
				certPathToStore = loginCertificate
			} else {
				// Client credentials flow — secret required
				if loginClientSecret == "" {
					// Try reading from stdin (for piped input)
					output.Info("Reading client secret from stdin...")
					var secret []byte
					secret = make([]byte, 1024)
					n, readErr := cmd.InOrStdin().Read(secret)
					if readErr != nil || n == 0 {
						return fmt.Errorf("--client-secret or --certificate is required for app-only mode")
					}
					loginClientSecret = strings.TrimSpace(string(secret[:n]))
				}

				output.Info(fmt.Sprintf("Authenticating profile %q via client credentials...", profileName))

				token, err := auth.LoginAppOnly(ctx, profile, loginClientSecret, ipv4Only)
				if err != nil {
					return fmt.Errorf("authentication failed: %w", err)
				}
				tokenStr = token.Token
				expiresOn = token.ExpiresOn
				clientSecretToStore = loginClientSecret

				profile.Username = "(app-only)"
			}
		} else {
			// Delegated: device-code flow with MSAL persistent cache.
			// MSAL stores both access and refresh tokens, enabling
			// silent renewal for ~90 days without user interaction.
			output.Info(fmt.Sprintf("Authenticating profile %q via device code flow (with persistent cache)...", profileName))

			result, loginErr := auth.LoginDelegatedWithCache(ctx, profile, ipv4Only, func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
				fmt.Println()
				fmt.Println(msg.Message)
				fmt.Println()
				return nil
			})
			if loginErr != nil {
				return fmt.Errorf("authentication failed: %w", loginErr)
			}
			tokenStr = result.Token.Token
			expiresOn = result.Token.ExpiresOn

			// Serialize the AuthenticationRecord for future silent refresh
			recordJSON, marshalErr := json.Marshal(result.AuthRecord)
			if marshalErr == nil {
				authRecordStr = string(recordJSON)
			}

			info, decodeErr := auth.DecodeTokenInfo(tokenStr)
			if decodeErr == nil && info.UPN != "" {
				profile.Username = info.UPN
			}
		}

		cache := &auth.TokenCache{
			AccessToken:  tokenStr,
			ClientSecret: clientSecretToStore,
			CertPath:     certPathToStore,
			AuthRecord:   authRecordStr,
			ExpiresAt:    expiresOn.Format(time.RFC3339),
			TokenType:    "Bearer",
			Scope:        fmt.Sprintf("%v", profile.Scopes),
		}
		if err := auth.StoreToken(profileName, cache); err != nil {
			return fmt.Errorf("storing token: %w", err)
		}

		cfg.Profiles[profileName] = profile
		cfg.ActiveProfile = profileName
		profile.Active = true
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		if format == output.FormatJSON {
			return output.JSON(map[string]interface{}{
				"profile":   profileName,
				"username":  profile.Username,
				"tenant":    profile.TenantID,
				"auth_mode": string(mode),
				"status":    "authenticated",
			})
		}

		output.Success(fmt.Sprintf("Authenticated as %s (profile: %s, mode: %s)", profile.Username, profileName, mode))
		return nil
	},
}

// --- auth status ---

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		profileName := flagProfile
		if profileName == "" {
			profileName = cfg.ActiveProfile
		}
		if profileName == "" {
			return fmt.Errorf("no active profile — run 'cb365 auth login' first")
		}

		profile, ok := cfg.Profiles[profileName]
		if !ok {
			return fmt.Errorf("profile %q not found", profileName)
		}

		cache, err := auth.LoadToken(profileName)
		if err != nil {
			return err
		}

		// Auto-refresh expired delegated tokens via MSAL persistent cache
		if profile.AuthMode == config.AuthModeDelegated {
			info, decodeErr := auth.DecodeTokenInfo(cache.AccessToken)
			if decodeErr != nil || info.IsExpired {
				cfg2, _ := config.Load()
				ipv4Only := auth.ShouldUseIPv4(cfg2)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				if flagVerbose {
					output.Info("Token expired \u2014 refreshing via MSAL cache...")
				}

				token, refreshErr := auth.GetTokenSilent(ctx, profile, cache.AuthRecord, profile.Scopes, ipv4Only)
				if refreshErr != nil {
					return fmt.Errorf("silent refresh failed: %w", refreshErr)
				}

				cache.AccessToken = token.Token
				cache.ExpiresAt = token.ExpiresOn.Format(time.RFC3339)
				_ = auth.StoreToken(profileName, cache)

				if flagVerbose {
					output.Success("Token refreshed successfully")
				}
			}
		}

		// Auto-refresh expired app-only tokens using stored client secret
		if profile.AuthMode == config.AuthModeAppOnly && cache.ClientSecret != "" {
			info, decodeErr := auth.DecodeTokenInfo(cache.AccessToken)
			if decodeErr != nil || info.IsExpired {
				cfg2, _ := config.Load()
				ipv4Only := auth.ShouldUseIPv4(cfg2)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				if flagVerbose {
					output.Info("Token expired — refreshing via client credentials...")
				}

				token, refreshErr := auth.RefreshAppOnly(ctx, profile, cache, ipv4Only)
				if refreshErr != nil {
					return fmt.Errorf("auto-refresh failed: %w", refreshErr)
				}

				cache.AccessToken = token.Token
				cache.ExpiresAt = token.ExpiresOn.Format(time.RFC3339)
				if err := auth.StoreToken(profileName, cache); err != nil {
					return fmt.Errorf("storing refreshed token: %w", err)
				}

				if flagVerbose {
					output.Success("Token refreshed successfully")
				}
			}
		}

		info, err := auth.DecodeTokenInfo(cache.AccessToken)
		if err != nil {
			return fmt.Errorf("decoding token: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"profile":    profileName,
				"auth_mode":  profile.AuthMode,
				"tenant_id":  info.TenantID,
				"username":   info.UPN,
				"name":       info.Name,
				"app":        info.AppName,
				"scopes":     info.Scopes,
				"expires_at": info.ExpiresAt,
				"valid_for":  info.ValidFor,
				"is_expired": info.IsExpired,
			})
		case output.FormatPlain:
			status := "valid"
			if info.IsExpired {
				status = "expired"
			}
			output.Plain([][]string{
				{profileName, string(profile.AuthMode), info.UPN, status, info.ValidFor},
			})
		default:
			status := "✓ Valid"
			if info.IsExpired {
				status = "✗ Expired"
			}
			fmt.Printf("Profile:   %s\n", profileName)
			fmt.Printf("Mode:      %s\n", profile.AuthMode)
			fmt.Printf("Tenant:    %s\n", info.TenantID)
			fmt.Printf("User:      %s (%s)\n", info.Name, info.UPN)
			fmt.Printf("App:       %s\n", info.AppName)
			fmt.Printf("Scopes:    %s\n", fmt.Sprintf("%v", info.Scopes))
			fmt.Printf("Expires:   %s\n", info.ExpiresAt)
			fmt.Printf("Status:    %s", status)
			if !info.IsExpired {
				fmt.Printf(" (%s remaining)", info.ValidFor)
			}
			fmt.Println()
		}
		return nil
	},
}

// --- auth logout ---

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove cached credentials for a profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		profileName := flagProfile
		if profileName == "" {
			profileName = cfg.ActiveProfile
		}
		if profileName == "" {
			return fmt.Errorf("no profile specified and no active profile set")
		}

		if err := auth.DeleteToken(profileName); err != nil {
			return err
		}

		delete(cfg.Profiles, profileName)
		if cfg.ActiveProfile == profileName {
			cfg.ActiveProfile = ""
		}
		if err := cfg.Save(); err != nil {
			return err
		}

		format := output.Resolve(flagJSON, flagPlain)
		if format == output.FormatJSON {
			return output.JSON(map[string]string{
				"profile": profileName,
				"status":  "logged_out",
			})
		}

		output.Success(fmt.Sprintf("Logged out of profile %q", profileName))
		return nil
	},
}

// --- auth profiles ---

var authProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List configured profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if len(cfg.Profiles) == 0 {
			output.Info("No profiles configured. Run 'cb365 auth login' to get started.")
			return nil
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(cfg.Profiles)
		case output.FormatPlain:
			var rows [][]string
			for _, p := range cfg.Profiles {
				active := ""
				if p.Active {
					active = "*"
				}
				rows = append(rows, []string{active, p.Name, string(p.AuthMode), p.TenantID, p.Username})
			}
			output.Plain(rows)
		default:
			headers := []string{"", "PROFILE", "MODE", "TENANT", "USER"}
			var rows [][]string
			for _, p := range cfg.Profiles {
				active := " "
				if p.Active {
					active = "●"
				}
				rows = append(rows, []string{active, p.Name, string(p.AuthMode), p.TenantID, p.Username})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// --- auth use ---

var authUseCmd = &cobra.Command{
	Use:   "use [profile]",
	Short: "Switch the active profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if err := cfg.SetActiveProfile(args[0]); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}

		format := output.Resolve(flagJSON, flagPlain)
		if format == output.FormatJSON {
			return output.JSON(map[string]string{
				"active_profile": args[0],
			})
		}

		output.Success(fmt.Sprintf("Switched to profile %q", args[0]))
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&loginTenant, "tenant", "", "Entra ID tenant ID or domain")
	authLoginCmd.Flags().StringVar(&loginClient, "client", "", "Entra ID application (client) ID")
	authLoginCmd.Flags().StringSliceVar(&loginScopes, "scopes", nil, "Graph API scopes (e.g. Tasks.ReadWrite,Mail.Read)")
	authLoginCmd.Flags().StringVar(&loginName, "name", "", "Profile name (default: 'default')")
	authLoginCmd.Flags().StringVar(&loginMode, "mode", "delegated", "Auth mode: delegated (device-code) or app-only (client credentials)")
	authLoginCmd.Flags().StringVar(&loginClientSecret, "client-secret", "", "Client secret for app-only mode (omit to read from stdin)")
	authLoginCmd.Flags().StringVar(&loginCertificate, "certificate", "", "Path to PEM file with certificate and private key (app-only mode)")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authProfilesCmd)
	authCmd.AddCommand(authUseCmd)
}



