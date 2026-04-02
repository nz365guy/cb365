package main

import (
	"context"
	"fmt"
	"time"

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
	loginTenant string
	loginClient string
	loginScopes []string
	loginName   string
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Entra ID (device-code flow)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if loginTenant == "" || loginClient == "" {
			return fmt.Errorf("--tenant and --client are required")
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
			AuthMode: config.AuthModeDelegated,
			Scopes:   loginScopes,
		}

		output.Info(fmt.Sprintf("Authenticating profile %q via device code flow...", profileName))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		token, err := auth.LoginDelegated(ctx, profile)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Decode token to get UPN for the profile
		info, err := auth.DecodeTokenInfo(token.Token)
		if err == nil && info.UPN != "" {
			profile.Username = info.UPN
		}

		// Store token in OS keychain
		cache := &auth.TokenCache{
			AccessToken: token.Token,
			ExpiresAt:   token.ExpiresOn.Format(time.RFC3339),
			TokenType:   "Bearer",
			Scope:       fmt.Sprintf("%v", profile.Scopes),
		}
		if err := auth.StoreToken(profileName, cache); err != nil {
			return fmt.Errorf("storing token: %w", err)
		}

		// Save profile to config
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
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
				"profile":  profileName,
				"username": profile.Username,
				"tenant":   profile.TenantID,
				"status":   "authenticated",
			})
		}

		output.Success(fmt.Sprintf("Authenticated as %s (profile: %s)", profile.Username, profileName))
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

		info, err := auth.DecodeTokenInfo(cache.AccessToken)
		if err != nil {
			return fmt.Errorf("decoding token: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]interface{}{
				"profile":   profileName,
				"auth_mode": profile.AuthMode,
				"tenant_id": info.TenantID,
				"username":  info.UPN,
				"name":      info.Name,
				"app":       info.AppName,
				"scopes":    info.Scopes,
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

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authProfilesCmd)
	authCmd.AddCommand(authUseCmd)
}
