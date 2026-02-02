package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"codeberg.org/brennenstuhl/uac-socializer/internal/app"
	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/infrastructure"
)

var (
	dryRun   bool
	dateFlag string
	platform string
)

// publishCmd represents the publish command
var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish anniversary blog posts to social media platforms",
	Long: `Publish historic anniversary blog posts to Bluesky and Mastodon with AI-generated content.

This command selects posts for the current or specified date from the UAC blog archive,
generates platform-optimized content via Mistral AI, and publishes to social platforms.

The content generation uses Mistral AI to create engaging, platform-specific posts that
reference the original blog post URLs while crafting community-focused messaging.

Examples:
  # Preview today's posts without publishing (recommended for testing)
  uac-socializer publish --dry-run

  # Publish to all platforms for today's anniversary posts
  uac-socializer publish

  # Publish specific date to Bluesky only
  uac-socializer publish --date 09-14 --platform bluesky

  # Preview content for a specific date
  uac-socializer publish --date 12-25 --dry-run --platform all

Environment Variables:
  UAC_MISTRAL_API_KEY       - Required: Mistral AI API key
  UAC_BLUESKY_HANDLE        - Required for Bluesky: your-handle.bsky.social
  UAC_BLUESKY_PASSWORD      - Required for Bluesky: app password
  UAC_MASTODON_ACCESS_TOKEN - Required for Mastodon: access token
  UAC_MASTODON_SERVER       - Required for Mastodon: instance URL (e.g., https://mastodon.social)
  UAC_LOG_LEVEL            - Optional: debug, info, warn, error (default: info)`,
	RunE: runPublishCommand,
}

func init() {
	rootCmd.AddCommand(publishCmd)

	// Add command flags
	publishCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview generated content without publishing (recommended for testing)")
	publishCmd.Flags().StringVar(&dateFlag, "date", "", "Target date in MM-DD format (default: today)")
	publishCmd.Flags().StringVar(&platform, "platform", "all", "Target platform: bluesky|mastodon|all")
}

// runPublishCommand executes the publish workflow with comprehensive error handling.
func runPublishCommand(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Config Loading
	config, err := app.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Container Initialization
	container, err := infrastructure.NewContainer(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to initialize container: %w", err)
	}

	logger := container.GetLogger()

	defer func() {
		if err := container.Close(); err != nil {
			logger.Error("failed to close container", slog.String("error", err.Error()))
		}
	}()

	// 3. Request Building
	publishRequest, err := buildPublishRequest(dateFlag, platform)
	if err != nil {
		return fmt.Errorf("invalid request parameters: %w", err)
	}

	logger.Debug("Publishing request built",
		slog.String("date", publishRequest.Date),
		slog.String("platform", publishRequest.Platform),
		slog.Bool("dry_run", dryRun),
	)

	// 4. Service Execution
	publishService := container.GetPublishService()

	publishServiceReq := &app.PublishRequest{
		DateKey:   publishRequest.Date,
		Platforms: parsePlatforms(publishRequest.Platform),
		DryRun:    dryRun,
	}

	response, err := publishService.PublishAnniversaryPosts(ctx, publishServiceReq)
	if err != nil {
		// Log the error but continue to show the result
		logger.Error("Publishing error", slog.String("error", err.Error()))
	}

	// 5. Output Formatting
	if err := formatRealOutput(response, dryRun); err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	// 6. Exit Codes
	if response != nil && response.Error != nil {
		return response.Error
	}

	return nil
}

// PublishRequest encapsulates the parameters for a publish operation.
type PublishRequest struct {
	Date     string // MM-DD format
	Platform string // bluesky, mastodon, or all
}

// buildPublishRequest validates and constructs a publish request from command flags.
func buildPublishRequest(dateInput, platformInput string) (*PublishRequest, error) {
	// Resolve date (use today if empty)
	date := dateInput
	if date == "" {
		date = time.Now().Format("01-02")
	}

	// Validate MM-DD format
	dateRegex := regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
	if !dateRegex.MatchString(date) {
		return nil, fmt.Errorf("invalid date format: %s (expected MM-DD, e.g., 09-14)", date)
	}

	// Validate platform
	plat := strings.ToLower(strings.TrimSpace(platformInput))
	switch plat {
	case domain.PlatformBluesky, domain.PlatformMastodon, domain.PlatformAll:
		// Valid platforms
	default:
		return nil, fmt.Errorf("invalid platform: %s (expected bluesky, mastodon, or all)", platformInput)
	}

	return &PublishRequest{
		Date:     date,
		Platform: plat,
	}, nil
}

// parsePlatforms converts a platform string to a slice of platform names.
func parsePlatforms(platformStr string) []string {
	platformStr = strings.ToLower(strings.TrimSpace(platformStr))
	if platformStr == domain.PlatformAll {
		return []string{domain.PlatformBluesky, domain.PlatformMastodon}
	}
	return []string{platformStr}
}

// formatRealOutput displays results from the real PublishService.
func formatRealOutput(response *app.PublishResponse, isDryRun bool) error {
	if response == nil {
		return fmt.Errorf("no response from publish service")
	}

	result := response.Result
	if result == nil {
		return fmt.Errorf("no result in response")
	}

	if isDryRun {
		return formatRealDryRunOutput(response)
	}
	return formatRealPublishOutput(response)
}

// formatRealDryRunOutput displays dry-run content from the real service.
func formatRealDryRunOutput(response *app.PublishResponse) error {
	result := response.Result
	fmt.Println("DRY RUN - Content Preview (no publishing)")
	fmt.Println("=" + strings.Repeat("=", 47))

	dateGroup := result.DateGroup()
	dateKey := dateGroup.DateKey()
	posts := dateGroup.Posts()

	fmt.Printf("Date: %s\n", dateKey)
	fmt.Printf("Posts found: %d\n", len(posts))

	if len(response.Generated) > 0 {
		fmt.Println("\nGenerated Content:")
		fmt.Println(strings.Repeat("-", 49))

		for platform, content := range response.Generated {
			fmt.Printf("\n%s (%d/%d chars | %d tokens | $%.4f):\n",
				strings.ToTitle(strings.ToLower(platform)),
				content.CharacterCount(),
				getCharacterLimit(platform),
				content.TokenCount(),
				content.Cost(),
			)
			fmt.Printf("  %s\n", content.Text())
		}
	} else {
		fmt.Println("No content generated (check error messages above)")
	}

	fmt.Printf("\n%s\n", strings.Repeat("-", 49))
	if result.TotalCost() > 0 {
		fmt.Printf("Total: $%.4f | %d tokens | Duration: %.1fs\n",
			result.TotalCost(),
			result.TotalTokens(),
			result.Duration().Seconds(),
		)
	} else {
		fmt.Printf("Duration: %.1fs\n", result.Duration().Seconds())
	}

	return nil
}

// formatRealPublishOutput displays actual publishing results.
func formatRealPublishOutput(response *app.PublishResponse) error {
	result := response.Result
	dateGroup := result.DateGroup()
	dateKey := dateGroup.DateKey()

	fmt.Printf("Publishing Anniversary Posts - %s\n", dateKey)
	fmt.Println("=" + strings.Repeat("=", 35))

	successCount := 0
	totalCount := result.SuccessCount() + result.FailureCount()

	for _, platformResult := range result.GetPlatformResults() {
		platform := platformResult.Platform()
		titlePlatform := capitalizeFirst(strings.ToLower(platform))

		switch platformResult.Status() {
		case domain.StatusSuccess:
			fmt.Printf("✅ %s: Published successfully\n", titlePlatform)
			if url := platformResult.PublishedURL(); url != "" {
				fmt.Printf("   URL: %s\n", url)
			}
			fmt.Printf("   Response: %.1fs\n", platformResult.ResponseTime().Seconds())
			successCount++
		case domain.StatusFailed:
			errMsg := "unknown error"
			if err := platformResult.Error(); err != nil {
				errMsg = err.Error()
			}
			fmt.Printf("❌ %s: %s\n", titlePlatform, errMsg)
		case domain.StatusSkipped:
			fmt.Printf("⏭️  %s: Skipped\n", titlePlatform)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("-", 40))
	if successCount == totalCount && totalCount > 0 {
		fmt.Printf("Success: %d/%d platforms | Cost: $%.4f | Duration: %.1fs\n",
			successCount, totalCount, result.TotalCost(), result.Duration().Seconds())
	} else if successCount > 0 {
		fmt.Printf("Partial: %d/%d succeeded | Duration: %.1fs | Exit: 1\n",
			successCount, totalCount, result.Duration().Seconds())
	} else {
		fmt.Printf("Failed: 0/%d succeeded | Duration: %.1fs | Exit: 1\n",
			totalCount, result.Duration().Seconds())
	}

	return nil
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	// Convert first rune to uppercase, keep the rest as-is
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// getCharacterLimit returns the character limit for the specified platform.
func getCharacterLimit(platform string) int {
	switch strings.ToLower(platform) {
	case domain.PlatformBluesky:
		return domain.BlueskyCharacterLimit
	case domain.PlatformMastodon:
		return domain.MastodonCharacterLimit
	default:
		return 280 // Default fallback
	}
}
