package main

import (
	"github.com/spf13/cobra"
)

var (
	flagJSON    bool
	flagPlain   bool
	flagProfile string
	flagVerbose bool
	flagDryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "cb365",
	Short: "Enterprise CLI for Microsoft 365 via Microsoft Graph",
	Long: `cb365 is an Entra ID-authenticated CLI for Microsoft 365.
Designed for agent consumption with structured JSON output.

Supports Microsoft To Do, Planner, Mail, Calendar, Contacts,
SharePoint, OneDrive, and Forms via the Microsoft Graph API.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output JSON to stdout")
	rootCmd.PersistentFlags().BoolVar(&flagPlain, "plain", false, "Output stable parseable text (TSV)")
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "Profile name override")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Preview write operations without executing")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(todoCmd)
	rootCmd.AddCommand(mailCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("cb365 version 0.1.0-dev")
		cmd.Println("https://github.com/nz365guy/cb365")
	},
}
