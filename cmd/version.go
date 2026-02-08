package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long: `Display the version, commit hash, and build date of the UAC Socializer application.

This information is useful for troubleshooting and verifying which version is deployed.

Example:
  uac-socializer version`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("UAC Socializer\n")
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Commit:     %s\n", Commit)
		fmt.Printf("  Build Date: %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
