package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Common result errors
var (
	ErrInvalidPlatform    = errors.New("invalid platform name")
	ErrEmptyGeneratedText = errors.New("generated text cannot be empty")
	ErrTextTooLong        = errors.New("generated text exceeds platform limits")
	ErrNegativeCost       = errors.New("cost cannot be negative")
	ErrNegativeTokens     = errors.New("token count cannot be negative")
)

// Platform constants for social media publishing
const (
	PlatformBluesky  = "bluesky"
	PlatformMastodon = "mastodon"
	PlatformAll      = "all"
)

// Platform-specific content limits
const (
	BlueskyCharacterLimit  = 300
	MastodonCharacterLimit = 500
)

// PublishStatus represents the outcome of a publishing operation
type PublishStatus string

const (
	StatusSuccess PublishStatus = "success"
	StatusFailed  PublishStatus = "failed"
	StatusSkipped PublishStatus = "skipped"
	StatusPending PublishStatus = "pending"
)

// GeneratedContent represents AI-generated content with platform-specific optimization.
// It encapsulates the output from content generation services like Mistral AI.
type GeneratedContent struct {
	text        string            // Generated post text
	platform    string            // Target platform (bluesky, mastodon)
	hashtags    []string          // Platform-appropriate hashtags
	metadata    map[string]string // Character count, engagement predictions, etc.
	promptUsed  string            // Original prompt for debugging/optimization
	generatedAt time.Time         // When content was generated
	tokenCount  int               // Number of tokens used in generation
	cost        float64           // Cost of generation in USD
}

// NewGeneratedContent creates a new GeneratedContent with validation.
func NewGeneratedContent(text, platform string, hashtags []string, promptUsed string, tokenCount int, cost float64) (*GeneratedContent, error) {
	const op = "NewGeneratedContent"

	// Validate platform
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !IsValidPlatform(platform) {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)}
	}

	// Validate text content
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, DomainError{Op: op, Err: ErrEmptyGeneratedText}
	}

	// Check platform-specific character limits
	if err := validateTextLength(text, platform); err != nil {
		return nil, DomainError{Op: op, Err: err}
	}

	// Validate token count
	if tokenCount < 0 {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: got %d", ErrNegativeTokens, tokenCount)}
	}

	// Validate cost
	if cost < 0 {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: got %f", ErrNegativeCost, cost)}
	}

	// Create metadata with basic metrics
	metadata := map[string]string{
		"character_count": fmt.Sprintf("%d", len(text)),
		"hashtag_count":   fmt.Sprintf("%d", len(hashtags)),
		"platform_limit":  fmt.Sprintf("%d", getPlatformLimit(platform)),
	}

	// Clean and validate hashtags
	cleanHashtags := make([]string, 0, len(hashtags))
	for _, tag := range hashtags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			// Ensure hashtag starts with #
			if !strings.HasPrefix(tag, "#") {
				tag = "#" + tag
			}
			cleanHashtags = append(cleanHashtags, tag)
		}
	}

	return &GeneratedContent{
		text:        text,
		platform:    platform,
		hashtags:    cleanHashtags,
		metadata:    metadata,
		promptUsed:  strings.TrimSpace(promptUsed),
		generatedAt: time.Now(),
		tokenCount:  tokenCount,
		cost:        cost,
	}, nil
}

// Text returns the generated post text.
func (gc *GeneratedContent) Text() string {
	return gc.text
}

// Platform returns the target platform.
func (gc *GeneratedContent) Platform() string {
	return gc.platform
}

// Hashtags returns a copy of the hashtags.
func (gc *GeneratedContent) Hashtags() []string {
	hashtags := make([]string, len(gc.hashtags))
	copy(hashtags, gc.hashtags)
	return hashtags
}

// Metadata returns a copy of the metadata map.
func (gc *GeneratedContent) Metadata() map[string]string {
	metadata := make(map[string]string)
	for k, v := range gc.metadata {
		metadata[k] = v
	}
	return metadata
}

// PromptUsed returns the original prompt used for generation.
func (gc *GeneratedContent) PromptUsed() string {
	return gc.promptUsed
}

// GeneratedAt returns when the content was generated.
func (gc *GeneratedContent) GeneratedAt() time.Time {
	return gc.generatedAt
}

// TokenCount returns the number of tokens used in generation.
func (gc *GeneratedContent) TokenCount() int {
	return gc.tokenCount
}

// Cost returns the cost of generation in USD.
func (gc *GeneratedContent) Cost() float64 {
	return gc.cost
}

// CharacterCount returns the number of characters in the generated text.
func (gc *GeneratedContent) CharacterCount() int {
	return len(gc.text)
}

// IsWithinLimit checks if the content fits within the platform's character limit.
func (gc *GeneratedContent) IsWithinLimit() bool {
	return gc.CharacterCount() <= getPlatformLimit(gc.platform)
}

// PlatformResult represents the outcome of publishing to a specific platform.
type PlatformResult struct {
	platform     string            // Platform name (bluesky, mastodon)
	status       PublishStatus     // Success/failure status
	content      *GeneratedContent // Generated content used
	publishedURL string            // URL of published post (if successful)
	error        error             // Error message (if failed)
	publishedAt  time.Time         // When publishing completed
	retryCount   int               // Number of retry attempts
	responseTime time.Duration     // Time taken for the publish operation
}

// NewPlatformResult creates a new PlatformResult.
func NewPlatformResult(platform string, status PublishStatus, content *GeneratedContent) (*PlatformResult, error) {
	const op = "NewPlatformResult"

	platform = strings.ToLower(strings.TrimSpace(platform))
	if !IsValidPlatform(platform) {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)}
	}

	return &PlatformResult{
		platform:     platform,
		status:       status,
		content:      content,
		publishedAt:  time.Now(),
		retryCount:   0,
		responseTime: 0,
	}, nil
}

// Platform returns the platform name.
func (pr *PlatformResult) Platform() string {
	return pr.platform
}

// Status returns the publish status.
func (pr *PlatformResult) Status() PublishStatus {
	return pr.status
}

// IsSuccess returns true if the publishing was successful.
func (pr *PlatformResult) IsSuccess() bool {
	return pr.status == StatusSuccess
}

// Content returns the generated content that was published.
func (pr *PlatformResult) Content() *GeneratedContent {
	return pr.content
}

// PublishedURL returns the URL of the published post.
func (pr *PlatformResult) PublishedURL() string {
	return pr.publishedURL
}

// Error returns the error that occurred during publishing.
func (pr *PlatformResult) Error() error {
	return pr.error
}

// PublishedAt returns when the publishing completed.
func (pr *PlatformResult) PublishedAt() time.Time {
	return pr.publishedAt
}

// RetryCount returns the number of retry attempts.
func (pr *PlatformResult) RetryCount() int {
	return pr.retryCount
}

// ResponseTime returns the time taken for the publish operation.
func (pr *PlatformResult) ResponseTime() time.Duration {
	return pr.responseTime
}

// SetSuccess updates the result with success information.
func (pr *PlatformResult) SetSuccess(publishedURL string, responseTime time.Duration) {
	pr.status = StatusSuccess
	pr.publishedURL = publishedURL
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// SetFailure updates the result with failure information.
func (pr *PlatformResult) SetFailure(err error, responseTime time.Duration) {
	pr.status = StatusFailed
	pr.error = err
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// SetSkipped updates the result with skipped information (e.g., for dry runs).
func (pr *PlatformResult) SetSkipped(reason string, responseTime time.Duration) {
	pr.status = StatusSkipped
	pr.publishedURL = reason
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// IncrementRetry increments the retry count.
func (pr *PlatformResult) IncrementRetry() {
	pr.retryCount++
}

// PublishResult aggregates publishing outcomes across multiple platforms.
// It provides a complete view of the social media publishing operation.
type PublishResult struct {
	post            *Post                      // Original post being published
	dateGroup       *DateGroup                 // Date group context
	platformResults map[string]*PlatformResult // Results per platform
	startTime       time.Time                  // When publishing started
	endTime         time.Time                  // When publishing completed
	totalCost       float64                    // Total AI generation cost
	totalTokens     int                        // Total tokens used across all platforms
	dryRun          bool                       // Whether this was a dry run
}

// NewPublishResult creates a new PublishResult.
func NewPublishResult(post *Post, dateGroup *DateGroup, dryRun bool) (*PublishResult, error) {
	const op = "NewPublishResult"

	if post == nil {
		return nil, DomainError{Op: op, Err: errors.New("post cannot be nil")}
	}

	if dateGroup == nil {
		return nil, DomainError{Op: op, Err: errors.New("dateGroup cannot be nil")}
	}

	return &PublishResult{
		post:            post,
		dateGroup:       dateGroup,
		platformResults: make(map[string]*PlatformResult),
		startTime:       time.Now(),
		dryRun:          dryRun,
	}, nil
}

// Post returns the original post.
func (pr *PublishResult) Post() *Post {
	return pr.post
}

// DateGroup returns the date group context.
func (pr *PublishResult) DateGroup() *DateGroup {
	return pr.dateGroup
}

// IsDryRun returns true if this was a dry run.
func (pr *PublishResult) IsDryRun() bool {
	return pr.dryRun
}

// AddPlatformResult adds a platform-specific result.
func (pr *PublishResult) AddPlatformResult(result *PlatformResult) error {
	if result == nil {
		return errors.New("platform result cannot be nil")
	}

	pr.platformResults[result.Platform()] = result

	// Update totals if content exists
	if content := result.Content(); content != nil {
		pr.totalCost += content.Cost()
		pr.totalTokens += content.TokenCount()
	}

	return nil
}

// GetPlatformResult returns the result for a specific platform.
func (pr *PublishResult) GetPlatformResult(platform string) *PlatformResult {
	return pr.platformResults[strings.ToLower(platform)]
}

// Platforms returns a list of all platforms that were attempted.
func (pr *PublishResult) Platforms() []string {
	platforms := make([]string, 0, len(pr.platformResults))
	for platform := range pr.platformResults {
		platforms = append(platforms, platform)
	}
	return platforms
}

// IsSuccess returns true if all platforms published successfully.
func (pr *PublishResult) IsSuccess() bool {
	if len(pr.platformResults) == 0 {
		return false
	}

	for _, result := range pr.platformResults {
		if !result.IsSuccess() {
			return false
		}
	}

	return true
}

// HasFailures returns true if any platform failed to publish.
func (pr *PublishResult) HasFailures() bool {
	for _, result := range pr.platformResults {
		if result.Status() == StatusFailed {
			return true
		}
	}
	return false
}

// SuccessCount returns the number of successful publishes.
func (pr *PublishResult) SuccessCount() int {
	count := 0
	for _, result := range pr.platformResults {
		if result.IsSuccess() {
			count++
		}
	}
	return count
}

// FailureCount returns the number of failed publishes.
func (pr *PublishResult) FailureCount() int {
	count := 0
	for _, result := range pr.platformResults {
		if result.Status() == StatusFailed {
			count++
		}
	}
	return count
}

// TotalCost returns the total AI generation cost across all platforms.
func (pr *PublishResult) TotalCost() float64 {
	return pr.totalCost
}

// TotalTokens returns the total tokens used across all platforms.
func (pr *PublishResult) TotalTokens() int {
	return pr.totalTokens
}

// Duration returns the total time taken for the publish operation.
func (pr *PublishResult) Duration() time.Duration {
	if pr.endTime.IsZero() {
		return time.Since(pr.startTime)
	}
	return pr.endTime.Sub(pr.startTime)
}

// Complete marks the publish operation as complete.
func (pr *PublishResult) Complete() {
	pr.endTime = time.Now()
}

// GetPlatformResults returns all platform results as a slice.
func (pr *PublishResult) GetPlatformResults() []*PlatformResult {
	results := make([]*PlatformResult, 0, len(pr.platformResults))
	for _, result := range pr.platformResults {
		results = append(results, result)
	}
	return results
}

// String returns a human-readable representation of the publish result.
func (pr *PublishResult) String() string {
	status := "success"
	if pr.HasFailures() {
		status = "partial"
		if pr.SuccessCount() == 0 {
			status = "failed"
		}
	}

	return fmt.Sprintf("PublishResult{Post: %s, Status: %s, Platforms: %d, Cost: $%.4f}",
		pr.post.URI(), status, len(pr.platformResults), pr.totalCost)
}

// Helper functions

// IsValidPlatform checks if a platform name is valid.
func IsValidPlatform(platform string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case PlatformBluesky, PlatformMastodon, PlatformAll:
		return true
	default:
		return false
	}
}

// validateTextLength checks if text fits within platform character limits.
func validateTextLength(text, platform string) error {
	limit := getPlatformLimit(platform)
	if len(text) > limit {
		return fmt.Errorf("%w: %d characters, limit is %d for %s", ErrTextTooLong, len(text), limit, platform)
	}
	return nil
}

// getPlatformLimit returns the character limit for a platform.
func getPlatformLimit(platform string) int {
	switch strings.ToLower(platform) {
	case PlatformBluesky:
		return BlueskyCharacterLimit
	case PlatformMastodon:
		return MastodonCharacterLimit
	default:
		return 280 // Default Twitter-like limit
	}
}
