package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/infrastructure"
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
  UAC_MASTODON_SERVER       - Required for Mastodon: instance URL (e.g., https://mastodon.social)`,
	RunE: runPublishCommand,
}

func init() {
	rootCmd.AddCommand(publishCmd)

	// Add command flags
	publishCmd.Flags().Bool("dry-run", false, "Preview generated content without publishing (recommended for testing)")
	publishCmd.Flags().String("date", "", "Target date in MM-DD format (default: today)")
	publishCmd.Flags().String("platform", "all", "Target platform: bluesky|mastodon|all")
}

// runPublishCommand executes the publish workflow with comprehensive error handling.
func runPublishCommand(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Extract flag values from Cobra's flag store
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	dateFlag, _ := cmd.Flags().GetString("date")
	platform, _ := cmd.Flags().GetString("platform")

	// 1. Config Loading
	config, err := app.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Initialize Logger (hard coded to JSON format, info level)
	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: infrastructure.SanitizeSecrets,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(handler))

	// 3. Container Initialization
	container, err := infrastructure.NewContainer(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to initialize container: %w", err)
	}

	logger := slog.Default()

	defer func() {
		if err := container.Close(); err != nil {
			logger.Error("failed to close container", slog.String("error", err.Error()))
		}
	}()

	// 4. Request Building
	publishRequest, err := buildPublishRequest(dateFlag, platform)
	if err != nil {
		return fmt.Errorf("invalid request parameters: %w", err)
	}

	logger.Debug("Publishing request built",
		slog.String("date", publishRequest.Date),
		slog.String("platform", publishRequest.Platform),
		slog.Bool("dry_run", dryRun),
	)

	// 5. Model check: refuse to run when the configured model is no longer served
	if err := container.GetContentGenerator().TestConnection(ctx); err != nil {
		return fmt.Errorf("content generator unavailable: %w", err)
	}

	// 6. Service Execution
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

	// 7. Output Formatting & Display Results
	if response != nil && response.Result != nil {
		result := response.Result
		if dryRun {
			// Dry-run mode: log preview of generated content
			post := result.Post()
			// Nil safety check for post
			if post == nil {
				logger.Warn("Post is nil in publish result")
			} else {
				logger.Info("Dry-run content preview",
					slog.String("mode", "dry-run"),
					slog.String("date", post.AnniversaryDateKey()),
					slog.String("uri", post.URI()))
			}

			if len(response.Generated) > 0 {
				logger.Info("Generated content available",
					slog.Int("platforms", len(response.Generated)))

				for platform, content := range response.Generated {
					logger.Info("Generated content for platform",
						slog.String("platform", platform),
						slog.Int("character_count", content.CharacterCount()),
						slog.Int("character_limit", getCharacterLimit(platform)),
						slog.String("content_text", content.Text()))
				}
			}

			// Summary for dry-run
			logger.Info("Dry-run summary",
				slog.Duration("duration", result.Duration()))
		} else {
			// Live mode: log actual publishing results
			post := result.Post()
			// Nil safety check for post
			if post == nil {
				logger.Warn("Post is nil in publish result")
			} else {
				logger.Info("Publishing anniversary posts",
					slog.String("date", post.AnniversaryDateKey()),
					slog.String("uri", post.URI()))
			}

			successCount := 0
			failureCount := 0
			skippedCount := 0

			for _, platformResult := range result.GetPlatformResults() {
				platform := platformResult.Platform()
				switch platformResult.Status() {
				case domain.StatusSuccess:
					logger.Info("Publishing result",
						slog.String("platform", platform),
						slog.String("status", "success"),
						slog.String("uri", platformResult.PublishedURL()),
						slog.Duration("response_time", platformResult.ResponseTime()))
					successCount++
				case domain.StatusFailed:
					errMsg := "unknown error"
					if err := platformResult.Error(); err != nil {
						errMsg = err.Error()
					}
					logger.Info("Publishing result",
						slog.String("platform", platform),
						slog.String("status", "failure"),
						slog.String("error", errMsg))
					failureCount++
				case domain.StatusSkipped:
					logger.Info("Publishing result",
						slog.String("platform", platform),
						slog.String("status", "skipped"))
					skippedCount++
				}
			}

			// Publishing summary
			totalCount := result.SuccessCount() + result.FailureCount()
			exitCode := 0
			if successCount < totalCount {
				exitCode = 1
			}

			logger.Info("Publishing summary",
				slog.Int("successful", successCount),
				slog.Int("failed", failureCount),
				slog.Int("skipped", skippedCount),
				slog.Int("total", totalCount),
				slog.Int("exit_code", exitCode),
				slog.Duration("total_duration", result.Duration()))
		}
	}

	// 8. Exit Codes
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
