/*
Copyright © 2026 UAC Socializer
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "uac-socializer",
	Short: "Publish historic Urban Art Core anniversary posts to social media with AI-generated content",
	Long: `UAC Socializer is a 12-factor Go CLI application that publishes historic anniversary
blog posts from Urban Art Core to Bluesky and Mastodon platforms.

The application reads posts from a curated archive, selects posts for anniversary dates,
generates platform-optimized content via Mistral AI, and publishes to social platforms
with rich formatting including hashtags, embeds, and community-focused messaging.

Key Features:
• AI-powered content generation using Mistral AI for engaging social media posts
• Platform-specific optimization (ATProto facets for Bluesky, hashtags for Mastodon)
• Anniversary-based posting from 15+ years of Urban Art Core blog history
• Dry-run mode for content preview and testing
• Comprehensive error handling and structured logging
• Containerized for serverless deployment

Configuration via Environment Variables:
  UAC_MISTRAL_API_KEY       - Mistral AI API key (required)
  UAC_BLUESKY_HANDLE        - Bluesky account handle (required for Bluesky)
  UAC_BLUESKY_PASSWORD      - Bluesky app password (required for Bluesky)
  UAC_MASTODON_ACCESS_TOKEN - Mastodon access token (required for Mastodon)
  UAC_MASTODON_SERVER       - Mastodon instance URL (required for Mastodon)
  UAC_LOG_LEVEL            - Logging level: debug, info, warn, error

Example Usage:
  # Preview today's anniversary posts
  uac-socializer publish --dry-run

  # Publish to all platforms
  uac-socializer publish

  # Publish specific date to Bluesky only
  uac-socializer publish --date 09-14 --platform bluesky

For more information visit: https://www.urbanartcore.com`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.uac-socializer.yaml)")

	// Global verbose flag for debugging
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose debug output")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".uac-socializer" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".uac-socializer")
	}

	viper.AutomaticEnv() // read in environment variables that match
}
