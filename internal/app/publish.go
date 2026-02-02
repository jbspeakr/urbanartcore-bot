package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// ApplicationError represents errors from the application layer.
type ApplicationError struct {
	Op  string // Operation that caused the error
	Err error  // Underlying error
}

func (e ApplicationError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("app.%s: %v", e.Op, e.Err)
}

func (e ApplicationError) Unwrap() error {
	return e.Err
}

// Common application errors
var (
	ErrInvalidDateKey   = errors.New("invalid date key format")
	ErrNoPostsFound     = errors.New("no posts found for the specified date")
	ErrNoPlatforms      = errors.New("no platforms available for publishing")
	ErrContextCancelled = errors.New("operation was cancelled")
	ErrCriticalFailure  = errors.New("critical failure stopped operation")
	ErrPartialFailure   = errors.New("some platforms failed to publish")
)

// PublishRequest contains all parameters needed for a publishing operation.
type PublishRequest struct {
	DateKey   string   // MM-DD format date key
	Platforms []string // Target platforms (bluesky, mastodon, all)
	DryRun    bool     // Whether to perform a dry run (generate but don't publish)
}

// PublishResponse contains the aggregated results of a publishing operation.
type PublishResponse struct {
	Result    *domain.PublishResult               // Aggregated publishing results
	Generated map[string]*domain.GeneratedContent // Generated content per platform
	Error     error                               // Any critical errors that occurred
}

// PublishService orchestrates the complete publishing workflow for anniversary posts.
// It coordinates between the repository, content generator, and social publishers
// to provide a unified publishing experience with proper error handling and logging.
type PublishService struct {
	repository ports.PostRepository
	generator  ports.ContentGenerator
	publishers map[string]ports.SocialPublisher
	logger     *slog.Logger
}

// NewPublishService creates a new PublishService with the provided dependencies.
func NewPublishService(
	repository ports.PostRepository,
	generator ports.ContentGenerator,
	publishers map[string]ports.SocialPublisher,
	logger *slog.Logger,
) *PublishService {
	return &PublishService{
		repository: repository,
		generator:  generator,
		publishers: publishers,
		logger:     logger,
	}
}

// PublishAnniversaryPosts executes the complete publishing workflow.
// It handles all 6 orchestration steps with proper error handling and context management.
func (ps *PublishService) PublishAnniversaryPosts(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	const op = "PublishAnniversaryPosts"

	ps.logger.Info("Starting anniversary post publishing",
		slog.String("dateKey", req.DateKey),
		slog.Any("platforms", req.Platforms),
		slog.Bool("dryRun", req.DryRun))

	// Step 1: Input Validation
	if err := ps.validateRequest(req); err != nil {
		return nil, ApplicationError{Op: op, Err: err}
	}

	// Step 2: Data Retrieval
	dateGroup, err := ps.repository.GetDateGroup(ctx, req.DateKey)
	if err != nil {
		if errors.Is(err, ports.ErrDateNotFound) {
			return nil, ApplicationError{Op: op, Err: ErrNoPostsFound}
		}
		return nil, ApplicationError{Op: op, Err: fmt.Errorf("failed to retrieve posts: %w", err)}
	}

	if dateGroup.IsEmpty() {
		return nil, ApplicationError{Op: op, Err: ErrNoPostsFound}
	}

	ps.logger.Info("Retrieved posts for anniversary date",
		slog.String("dateKey", req.DateKey),
		slog.Int("postCount", dateGroup.Count()))

	// Step 3: Platform Resolution
	platforms, err := ps.resolvePlatforms(req.Platforms)
	if err != nil {
		return nil, ApplicationError{Op: op, Err: err}
	}

	if len(platforms) == 0 {
		return nil, ApplicationError{Op: op, Err: ErrNoPlatforms}
	}

	ps.logger.Info("Resolved target platforms", slog.Any("platforms", platforms))

	// Initialize result tracking
	posts := dateGroup.Posts()
	publishResult, err := domain.NewPublishResult(posts[0], dateGroup, req.DryRun)
	if err != nil {
		return nil, ApplicationError{Op: op, Err: fmt.Errorf("failed to create publish result: %w", err)}
	}

	generatedContent := make(map[string]*domain.GeneratedContent)

	// Step 4: Content Generation Loop
	for _, platform := range platforms {
		select {
		case <-ctx.Done():
			ps.logger.Warn("Content generation cancelled by context")
			return nil, ApplicationError{Op: op, Err: ErrContextCancelled}
		default:
		}

		ps.logger.Info("Generating content for platform", slog.String("platform", platform))

		content, err := ps.generateContentForPlatform(ctx, posts, platform, req.DateKey, req.DryRun)
		if err != nil {
			// Platform error - continue with other platforms
			ps.logger.Error("Content generation failed for platform",
				slog.String("platform", platform),
				slog.String("error", err.Error()))

			// Create failed platform result
			platformResult, _ := domain.NewPlatformResult(platform, domain.StatusFailed, nil)
			platformResult.SetFailure(fmt.Errorf("content generation failed: %w", err), 0)
			if err := publishResult.AddPlatformResult(platformResult); err != nil {
				ps.logger.Error("Failed to add platform result", slog.String("error", err.Error()))
			}
			continue
		}

		generatedContent[platform] = content
		ps.logger.Info("Content generated successfully",
			slog.String("platform", platform),
			slog.Int("characters", content.CharacterCount()),
			slog.Int("tokens", content.TokenCount()))
	}

	// Check if we have any content to publish
	if len(generatedContent) == 0 {
		return nil, ApplicationError{Op: op, Err: errors.New("failed to generate content for any platform")}
	}

	// Step 5: Publishing Loop
	for platform, content := range generatedContent {
		select {
		case <-ctx.Done():
			ps.logger.Warn("Publishing cancelled by context")
			return nil, ApplicationError{Op: op, Err: ErrContextCancelled}
		default:
		}

		ps.logger.Info("Publishing to platform", slog.String("platform", platform))

		platformResult, err := ps.publishToPlatform(ctx, platform, content, req.DryRun)
		if err != nil {
			// Platform error - continue with other platforms
			ps.logger.Error("Publishing failed for platform",
				slog.String("platform", platform),
				slog.String("error", err.Error()))

			if platformResult == nil {
				platformResult, _ = domain.NewPlatformResult(platform, domain.StatusFailed, content)
				platformResult.SetFailure(err, 0)
			}
		}

		if err := publishResult.AddPlatformResult(platformResult); err != nil {
			ps.logger.Error("Failed to add platform result", slog.String("error", err.Error()))
		}
	}

	// Step 6: Result Aggregation
	publishResult.Complete()

	ps.logger.Info("Publishing operation completed",
		slog.Int("successCount", publishResult.SuccessCount()),
		slog.Int("failureCount", publishResult.FailureCount()),
		slog.Float64("totalCost", publishResult.TotalCost()),
		slog.Int("totalTokens", publishResult.TotalTokens()),
		slog.Duration("duration", publishResult.Duration()))

	response := &PublishResponse{
		Result:    publishResult,
		Generated: generatedContent,
	}

	// Determine if we had partial failures
	if publishResult.HasFailures() {
		if publishResult.SuccessCount() == 0 {
			response.Error = ErrCriticalFailure
		} else {
			response.Error = ErrPartialFailure
		}
	}

	return response, nil
}

// validateRequest validates the publishing request parameters.
func (ps *PublishService) validateRequest(req *PublishRequest) error {
	if req == nil {
		return errors.New("request cannot be nil")
	}

	// Validate date key format (MM-DD)
	dateKey := strings.TrimSpace(req.DateKey)
	if dateKey == "" {
		return fmt.Errorf("%w: date key cannot be empty", ErrInvalidDateKey)
	}

	dateKeyPattern := regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
	if !dateKeyPattern.MatchString(dateKey) {
		return fmt.Errorf("%w: expected MM-DD format, got %q", ErrInvalidDateKey, dateKey)
	}

	// Validate platforms
	if len(req.Platforms) == 0 {
		return errors.New("at least one platform must be specified")
	}

	for _, platform := range req.Platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if !domain.IsValidPlatform(platform) {
			return fmt.Errorf("invalid platform: %s", platform)
		}
	}

	return nil
}

// resolvePlatforms converts platform names to normalized platform list.
func (ps *PublishService) resolvePlatforms(platforms []string) ([]string, error) {
	resolved := make([]string, 0)

	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))

		if platform == domain.PlatformAll {
			// Expand "all" to all available publishers
			for publisherPlatform := range ps.publishers {
				resolved = append(resolved, publisherPlatform)
			}
		} else {
			// Check if we have a publisher for this platform
			if _, exists := ps.publishers[platform]; exists {
				resolved = append(resolved, platform)
			} else {
				ps.logger.Warn("Publisher not available for platform", slog.String("platform", platform))
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, platform := range resolved {
		if !seen[platform] {
			unique = append(unique, platform)
			seen[platform] = true
		}
	}

	return unique, nil
}

// generateContentForPlatform generates content for a specific platform.
func (ps *PublishService) generateContentForPlatform(ctx context.Context, posts []*domain.Post, platform, dateKey string, dryRun bool) (*domain.GeneratedContent, error) {
	req, err := ports.NewGenerationRequest(posts, platform, dateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create generation request: %w", err)
	}

	// Add anniversary context
	req.WithContext("anniversary_date", dateKey)
	req.WithContext("post_count", fmt.Sprintf("%d", len(posts)))

	if dryRun {
		// In dry-run mode, placeholder generator will return "not implemented" error
		// We catch this and create mock content for preview
		content, err := ps.generator.GenerateContent(ctx, req)
		if err != nil {
			if strings.Contains(err.Error(), "not implemented") {
				ps.logger.Info("DRY RUN - content generation simulated", slog.String("platform", platform))
				return ps.createMockContent(platform, posts, dateKey), nil
			}
			return nil, err
		}
		return content, nil
	}

	return ps.generator.GenerateContent(ctx, req)
}

// publishToPlatform publishes content to a specific platform.
func (ps *PublishService) publishToPlatform(ctx context.Context, platform string, content *domain.GeneratedContent, dryRun bool) (*domain.PlatformResult, error) {
	publisher, exists := ps.publishers[platform]
	if !exists {
		return nil, fmt.Errorf("no publisher available for platform: %s", platform)
	}

	if dryRun {
		ps.logger.Info("DRY RUN - skipping actual publish", slog.String("platform", platform))

		// Validate content in dry-run mode, but expect "not implemented" error
		if err := publisher.ValidateContent(ctx, content); err != nil {
			if !strings.Contains(err.Error(), "not implemented") {
				return nil, fmt.Errorf("content validation failed: %w", err)
			}
		}

		// Create dry-run result with skipped status
		result, _ := domain.NewPlatformResult(platform, domain.StatusSkipped, content)
		result.SetSkipped("dry-run-"+platform, 0)
		return result, nil
	}

	// Validate content before publishing
	if err := publisher.ValidateContent(ctx, content); err != nil {
		result, _ := domain.NewPlatformResult(platform, domain.StatusFailed, content)
		result.SetFailure(fmt.Errorf("content validation failed: %w", err), 0)
		return result, err
	}

	// Perform actual publish
	startTime := time.Now()
	result, err := publisher.Publish(ctx, content)
	responseTime := time.Since(startTime)

	if err != nil {
		if result == nil {
			result, _ = domain.NewPlatformResult(platform, domain.StatusFailed, content)
		}
		result.SetFailure(err, responseTime)
		return result, err
	}

	return result, nil
}

// createMockContent creates mock content for dry-run mode.
func (ps *PublishService) createMockContent(platform string, posts []*domain.Post, dateKey string) *domain.GeneratedContent {
	mockText := fmt.Sprintf("🎉 Anniversary post for %s! Celebrating %d year(s) of Urban Art Core content. #UrbanArt #Anniversary",
		dateKey, time.Now().Year()-posts[0].Date().Year())

	// Truncate if too long for platform
	limit := domain.BlueskyCharacterLimit
	if platform == domain.PlatformMastodon {
		limit = domain.MastodonCharacterLimit
	}

	if len(mockText) > limit {
		mockText = mockText[:limit-3] + "..."
	}

	hashtags := []string{"#UrbanArt", "#Anniversary", "#StreetArt"}
	content, _ := domain.NewGeneratedContent(mockText, platform, hashtags, "mock-prompt-for-dry-run", 100, 0.01)

	return content
}
