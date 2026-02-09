package domain

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"
)

// Common result errors
var (
	ErrInvalidPlatform    = errors.New("invalid platform name")
	ErrEmptyGeneratedText = errors.New("generated text cannot be empty")
	ErrTextTooLong        = errors.New("generated text exceeds platform limits")
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
}

// NewGeneratedContent creates a new GeneratedContent with validation.
func NewGeneratedContent(text, platform string, hashtags []string, promptUsed string) (*GeneratedContent, error) {
	// Validate platform
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !IsValidPlatform(platform) {
		return nil, DomainError{Err: fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)}
	}

	// Validate text content
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, DomainError{Err: ErrEmptyGeneratedText}
	}

	// Check platform-specific character limits
	if err := validateTextLength(text, platform); err != nil {
		return nil, DomainError{Err: err}
	}

	// Create metadata with basic metrics
	// Note: character_count is platform-specific (bytes for Bluesky, runes for Mastodon)
	metadata := map[string]string{
		"character_count": fmt.Sprintf("%d", countTextLength(text, platform)),
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
	}, nil
}

// Text returns the generated post text.
func (gc *GeneratedContent) Text() string {
	if gc == nil {
		return ""
	}
	return gc.text
}

// Platform returns the target platform.
func (gc *GeneratedContent) Platform() string {
	if gc == nil {
		return ""
	}
	return gc.platform
}

// Hashtags returns a copy of the hashtags.
func (gc *GeneratedContent) Hashtags() []string {
	if gc == nil {
		return []string{}
	}
	hashtags := make([]string, len(gc.hashtags))
	copy(hashtags, gc.hashtags)
	return hashtags
}

// Metadata returns a copy of the metadata map.
func (gc *GeneratedContent) Metadata() map[string]string {
	if gc == nil {
		return map[string]string{}
	}
	return maps.Clone(gc.metadata)
}

// AddMetadata adds a key-value pair to the metadata map.
// This allows adapters to enrich content with additional context.
func (gc *GeneratedContent) AddMetadata(key, value string) {
	if gc == nil {
		return
	}
	if gc.metadata == nil {
		gc.metadata = make(map[string]string)
	}
	gc.metadata[key] = value
}

// PromptUsed returns the original prompt used for generation.
func (gc *GeneratedContent) PromptUsed() string {
	if gc == nil {
		return ""
	}
	return gc.promptUsed
}

// GeneratedAt returns when the content was generated.
func (gc *GeneratedContent) GeneratedAt() time.Time {
	if gc == nil {
		return time.Time{}
	}
	return gc.generatedAt
}

// CharacterCount returns the byte count of the generated text.
// For platform-aware counting, use Metadata() to access the "character_count" field
// or IsWithinLimit() to check if content fits the platform's character limit.
func (gc *GeneratedContent) CharacterCount() int {
	if gc == nil {
		return 0
	}
	return len(gc.text)
}

// IsWithinLimit checks if the content fits within the platform's character limit.
// Uses platform-specific counting: bytes for Bluesky, runes for Mastodon.
func (gc *GeneratedContent) IsWithinLimit() bool {
	if gc == nil {
		return false
	}
	limit := getPlatformLimit(gc.platform)
	currentLength := countTextLength(gc.text, gc.platform)
	return currentLength <= limit
}

// PlatformResult represents the outcome of publishing to a specific platform.
type PlatformResult struct {
	platform     string            // Platform name (bluesky, mastodon)
	status       PublishStatus     // Success/failure status
	content      *GeneratedContent // Generated content used
	publishedURL string            // URL of published post (if successful)
	error        error             // Error message (if failed)
	skipReason   string            // Reason for skipping (e.g., dry-run mode)
	publishedAt  time.Time         // When publishing completed
	retryCount   int               // Number of retry attempts
	responseTime time.Duration     // Time taken for the publish operation
}

// NewPlatformResult creates a new PlatformResult.
func NewPlatformResult(platform string, status PublishStatus, content *GeneratedContent) (*PlatformResult, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !IsValidPlatform(platform) {
		return nil, DomainError{Err: fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)}
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
	if pr == nil {
		return ""
	}
	return pr.platform
}

// Status returns the publish status.
func (pr *PlatformResult) Status() PublishStatus {
	if pr == nil {
		return StatusPending
	}
	return pr.status
}

// IsSuccess returns true if the publishing was successful.
func (pr *PlatformResult) IsSuccess() bool {
	if pr == nil {
		return false
	}
	return pr.status == StatusSuccess
}

// Content returns the generated content that was published.
func (pr *PlatformResult) Content() *GeneratedContent {
	if pr == nil {
		return nil
	}
	return pr.content
}

// PublishedURL returns the URL of the published post.
func (pr *PlatformResult) PublishedURL() string {
	if pr == nil {
		return ""
	}
	return pr.publishedURL
}

// Error returns the error that occurred during publishing.
func (pr *PlatformResult) Error() error {
	if pr == nil {
		return nil
	}
	return pr.error
}

// SkipReason returns the reason for skipping (e.g., for dry runs).
func (pr *PlatformResult) SkipReason() string {
	if pr == nil {
		return ""
	}
	return pr.skipReason
}

// PublishedAt returns when the publishing completed.
func (pr *PlatformResult) PublishedAt() time.Time {
	if pr == nil {
		return time.Time{}
	}
	return pr.publishedAt
}

// RetryCount returns the number of retry attempts.
func (pr *PlatformResult) RetryCount() int {
	if pr == nil {
		return 0
	}
	return pr.retryCount
}

// ResponseTime returns the time taken for the publish operation.
func (pr *PlatformResult) ResponseTime() time.Duration {
	if pr == nil {
		return 0
	}
	return pr.responseTime
}

// SetSuccess updates the result with success information.
func (pr *PlatformResult) SetSuccess(publishedURL string, responseTime time.Duration) {
	if pr == nil {
		return
	}
	pr.status = StatusSuccess
	pr.publishedURL = publishedURL
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// SetFailure updates the result with failure information.
func (pr *PlatformResult) SetFailure(err error, responseTime time.Duration) {
	if pr == nil {
		return
	}
	pr.status = StatusFailed
	pr.error = err
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// SetSkipped updates the result with skipped information (e.g., for dry runs).
// reason should describe why the publish was skipped.
func (pr *PlatformResult) SetSkipped(reason string, responseTime time.Duration) {
	if pr == nil {
		return
	}
	pr.status = StatusSkipped
	pr.skipReason = reason
	pr.responseTime = responseTime
	pr.publishedAt = time.Now()
}

// IncrementRetry increments the retry count up to a maximum bound.
// Returns true if the retry count was incremented, false if at maximum.
// maxRetries specifies the maximum allowed retry attempts (must be >= 0).
func (pr *PlatformResult) IncrementRetry(maxRetries int) bool {
	if pr == nil {
		return false
	}
	// Enforce maximum to prevent excessive retries
	if maxRetries < 0 {
		return false // Invalid max retries
	}
	if pr.retryCount >= maxRetries {
		return false
	}
	pr.retryCount++
	return true
}

// PublishResult aggregates publishing outcomes across multiple platforms.
// It provides a complete view of the social media publishing operation.
type PublishResult struct {
	post            *Post                      // Original post being published
	platformResults map[string]*PlatformResult // Results per platform
	startTime       time.Time                  // When publishing started
	endTime         time.Time                  // When publishing completed
	dryRun          bool                       // Whether this was a dry run
}

// NewPublishResult creates a new PublishResult.
func NewPublishResult(post *Post, dryRun bool) (*PublishResult, error) {
	if post == nil {
		return nil, DomainError{Err: errors.New("post cannot be nil")}
	}

	return &PublishResult{
		post:            post,
		platformResults: make(map[string]*PlatformResult),
		startTime:       time.Now(),
		dryRun:          dryRun,
	}, nil
}

// Post returns the original post.
func (pr *PublishResult) Post() *Post {
	if pr == nil {
		return nil
	}
	return pr.post
}

// IsDryRun returns true if this was a dry run.
func (pr *PublishResult) IsDryRun() bool {
	if pr == nil {
		return false
	}
	return pr.dryRun
}

// AddPlatformResult adds a platform-specific result.
func (pr *PublishResult) AddPlatformResult(result *PlatformResult) error {
	if result == nil {
		return errors.New("platform result cannot be nil")
	}

	pr.platformResults[result.Platform()] = result
	return nil
}

// GetPlatformResult returns the result for a specific platform.
func (pr *PublishResult) GetPlatformResult(platform string) *PlatformResult {
	if pr == nil {
		return nil
	}
	return pr.platformResults[strings.ToLower(platform)]
}

// Platforms returns a list of all platforms that were attempted.
func (pr *PublishResult) Platforms() []string {
	if pr == nil {
		return []string{}
	}
	platforms := make([]string, 0, len(pr.platformResults))
	for platform := range pr.platformResults {
		platforms = append(platforms, platform)
	}
	return platforms
}

// IsSuccess returns true if all platforms published successfully.
func (pr *PublishResult) IsSuccess() bool {
	if pr == nil || len(pr.platformResults) == 0 {
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
	if pr == nil {
		return false
	}
	for _, result := range pr.platformResults {
		if result.Status() == StatusFailed {
			return true
		}
	}
	return false
}

// SuccessCount returns the number of successful publishes.
func (pr *PublishResult) SuccessCount() int {
	if pr == nil {
		return 0
	}
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
	if pr == nil {
		return 0
	}
	count := 0
	for _, result := range pr.platformResults {
		if result.Status() == StatusFailed {
			count++
		}
	}
	return count
}

// Duration returns the total time taken for the publish operation.
func (pr *PublishResult) Duration() time.Duration {
	if pr == nil {
		return 0
	}
	if pr.endTime.IsZero() {
		return time.Since(pr.startTime)
	}
	return pr.endTime.Sub(pr.startTime)
}

// Complete marks the publish operation as complete.
func (pr *PublishResult) Complete() {
	if pr == nil {
		return
	}
	pr.endTime = time.Now()
}

// GetPlatformResults returns all platform results as a slice.
func (pr *PublishResult) GetPlatformResults() []*PlatformResult {
	if pr == nil {
		return []*PlatformResult{}
	}
	results := make([]*PlatformResult, 0, len(pr.platformResults))
	for _, result := range pr.platformResults {
		results = append(results, result)
	}
	return results
}

// String returns a human-readable representation of the publish result.
func (pr *PublishResult) String() string {
	if pr == nil {
		return "PublishResult{nil}"
	}

	status := "success"
	if pr.HasFailures() {
		status = "partial"
		if pr.SuccessCount() == 0 {
			status = "failed"
		}
	}

	postURI := "unknown"
	if pr.post != nil {
		postURI = pr.post.URI()
	}

	return fmt.Sprintf("PublishResult{Post: %s, Status: %s, Platforms: %d}",
		postURI, status, len(pr.platformResults))
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

// countTextLength returns the length based on platform spec.
// Bluesky uses byte count (UTF-8), Mastodon uses rune count (characters).
func countTextLength(text, platform string) int {
	switch strings.ToLower(platform) {
	case PlatformBluesky:
		return len(text) // UTF-8 bytes
	case PlatformMastodon:
		return len([]rune(text)) // Character count
	default:
		return len(text)
	}
}

// validateTextLength checks if text fits within platform character limits.
// For Bluesky: counts UTF-8 bytes
// For Mastodon: counts runes (characters)
func validateTextLength(text, platform string) error {
	limit := getPlatformLimit(platform)
	currentLength := countTextLength(text, platform)

	if currentLength > limit {
		var unit string
		switch strings.ToLower(platform) {
		case PlatformBluesky:
			unit = "bytes"
		case PlatformMastodon:
			unit = "characters"
		default:
			unit = "characters"
		}

		return fmt.Errorf("%w: %d %s, limit is %d for %s", ErrTextTooLong, currentLength, unit, limit, platform)
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
