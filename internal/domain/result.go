package domain

import (
	"crypto/sha256"
	"encoding/hex"
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
)

// Platform constants for social media publishing
const (
	PlatformBluesky  = "bluesky"
	PlatformMastodon = "mastodon"
	PlatformAll      = "all"
)

// Platform-specific content limits, in the units each platform counts:
// grapheme clusters for Bluesky, characters with URLs as 23 for Mastodon.
// Bluesky additionally caps the byte length at BlueskyByteLimit.
const (
	BlueskyCharacterLimit  = BlueskyGraphemeLimit
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
	text        string    // Generated post text
	platform    string    // Target platform (bluesky, mastodon)
	hashtags    []string  // Platform-appropriate hashtags
	uri         string    // Source blog post URI (required for embeds/attribution)
	promptUsed  string    // Original prompt for debugging/optimization
	generatedAt time.Time // When content was generated
	claims      []string  // Factual statements the text makes, as the writer lists them
	fallback    bool      // Built from the template, not generated
}

// NewGeneratedContent creates a new GeneratedContent with validation.
func NewGeneratedContent(text, platform string, hashtags []string, promptUsed, uri string) (*GeneratedContent, error) {
	// Validate URI
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, DomainError{Err: errors.New("uri cannot be empty - all content must have a source")}
	}

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

	// Check platform-specific limits on the text as it will be posted,
	// with hashtags appended and, on Bluesky, URLs removed
	if err := validateTextLength(RenderText(text, platform, cleanHashtags), platform); err != nil {
		return nil, DomainError{Err: err}
	}

	return &GeneratedContent{
		text:        text,
		platform:    platform,
		hashtags:    cleanHashtags,
		uri:         uri,
		promptUsed:  strings.TrimSpace(promptUsed),
		generatedAt: time.Now(),
	}, nil
}

// WithClaims records the factual statements the text makes, for review.
func (gc *GeneratedContent) WithClaims(claims []string) *GeneratedContent {
	if gc == nil {
		return nil
	}
	gc.claims = make([]string, 0, len(claims))
	for _, claim := range claims {
		if claim = strings.TrimSpace(claim); claim != "" {
			gc.claims = append(gc.claims, claim)
		}
	}
	return gc
}

// Claims returns a copy of the factual statements the text makes.
func (gc *GeneratedContent) Claims() []string {
	if gc == nil {
		return nil
	}
	return append([]string(nil), gc.claims...)
}

// IsFallback reports whether the content was built from the fallback
// template rather than generated.
func (gc *GeneratedContent) IsFallback() bool {
	return gc != nil && gc.fallback
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

// URI returns the source blog post URL for embeds/attribution.
func (gc *GeneratedContent) URI() string {
	if gc == nil {
		return ""
	}
	return gc.uri
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

// RenderedText returns the text as it will be posted on the platform,
// with hashtags appended and, on Bluesky, URLs removed.
func (gc *GeneratedContent) RenderedText() string {
	if gc == nil {
		return ""
	}
	return RenderText(gc.text, gc.platform, gc.hashtags)
}

// CharacterCount returns the length of the rendered text as the platform counts it:
// grapheme clusters for Bluesky, characters with URLs as 23 for Mastodon.
func (gc *GeneratedContent) CharacterCount() int {
	if gc == nil {
		return 0
	}
	return countTextLength(gc.RenderedText(), gc.platform)
}

// IsWithinLimit checks if the rendered content fits within the platform's limits.
func (gc *GeneratedContent) IsWithinLimit() bool {
	if gc == nil {
		return false
	}
	return validateTextLength(gc.RenderedText(), gc.platform) == nil
}

// IdempotencyKey returns a stable key for publishing this content on its
// platform on the day it was generated. A rerun on the same day for the same
// post yields the same key, so platforms that honour idempotency keys drop
// the duplicate.
func (gc *GeneratedContent) IdempotencyKey() string {
	if gc == nil {
		return ""
	}
	raw := gc.generatedAt.UTC().Format(time.DateOnly) + "/" + gc.uri + "/" + gc.platform
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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

// countTextLength returns the length of text as the platform counts it.
// Bluesky counts grapheme clusters, Mastodon counts characters with every
// URL as MastodonURLLength.
func countTextLength(text, platform string) int {
	switch strings.ToLower(platform) {
	case PlatformBluesky:
		return CountGraphemes(text)
	case PlatformMastodon:
		return CountMastodonCharacters(text)
	default:
		return len([]rune(text))
	}
}

// validateTextLength checks if text fits within platform limits.
// For Bluesky: 300 grapheme clusters and 3,000 UTF-8 bytes
// For Mastodon: 500 characters, URLs counted as 23
func validateTextLength(text, platform string) error {
	limit := getPlatformLimit(platform)
	currentLength := countTextLength(text, platform)

	if currentLength > limit {
		unit := "characters"
		if strings.ToLower(platform) == PlatformBluesky {
			unit = "graphemes"
		}
		return fmt.Errorf("%w: %d %s, limit is %d for %s", ErrTextTooLong, currentLength, unit, limit, platform)
	}

	if strings.ToLower(platform) == PlatformBluesky && len(text) > BlueskyByteLimit {
		return fmt.Errorf("%w: %d bytes, limit is %d for %s", ErrTextTooLong, len(text), BlueskyByteLimit, platform)
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
