package ports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
)

// PublisherError represents social media platform publishing errors.
type PublisherError struct {
	Platform  string            // Platform where error occurred
	Err       error             // Underlying error
	Retryable bool              // Whether this error can be retried
	Metadata  map[string]string // Additional error context
}

func (e PublisherError) Error() string {
	platform := e.Platform
	if platform == "" {
		platform = "unknown"
	}
	return fmt.Sprintf("publisher[%s]: %v", platform, e.Err)
}

func (e PublisherError) Unwrap() error {
	return e.Err
}

// IsRetryable returns whether this error indicates a retryable condition.
func (e PublisherError) IsRetryable() bool {
	return e.Retryable
}

// Common publisher errors
var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrContentTooLong       = errors.New("content exceeds platform character limit")
	ErrContentRejected      = errors.New("content rejected by platform")
	ErrNetworkTimeout       = errors.New("network timeout")
	ErrPlatformUnavailable  = errors.New("platform service unavailable")
	ErrInvalidCredentials   = errors.New("invalid or expired credentials")
	ErrDuplicatePost        = errors.New("duplicate post detected")
)

// SocialPublisher defines the contract for publishing content to social media platforms.
// Each implementation handles platform-specific authentication, formatting,
// and error handling while providing a consistent interface.
type SocialPublisher interface {
	// Platform returns the platform this publisher targets.
	Platform() string

	// Publish sends content to the social media platform.
	// The content should already be optimized for this specific platform.
	// Returns a PlatformResult containing success/failure status and metadata.
	Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error)

	// ValidateContent checks if content meets platform-specific requirements
	// before attempting to publish. This includes character limits, formatting,
	// and content policy validation.
	ValidateContent(ctx context.Context, content *domain.GeneratedContent) error

	// GetCharacterLimit returns the maximum character count for this platform.
	// Used by content generators to optimize text length.
	GetCharacterLimit() int

	// SupportsHashtags returns whether this platform supports hashtag functionality.
	SupportsHashtags() bool

	// SupportsMedia returns whether this platform supports media attachments.
	SupportsMedia() bool

	// TestConnection verifies that the publisher can connect to the platform
	// with current credentials. Used during application startup and health checks.
	TestConnection(ctx context.Context) error
}

// PublishOptions provides additional configuration for publishing operations.
type PublishOptions struct {
	DryRun     bool              `json:"dry_run"`     // Don't actually publish
	RetryCount int               `json:"retry_count"` // Number of retry attempts
	Timeout    time.Duration     `json:"timeout"`     // Publishing timeout
	Metadata   map[string]string `json:"metadata"`    // Additional context
}

// DefaultPublishOptions returns sensible defaults for publish operations.
func DefaultPublishOptions() *PublishOptions {
	return &PublishOptions{
		DryRun:     false,
		RetryCount: 3,
		Timeout:    30 * time.Second,
		Metadata:   make(map[string]string),
	}
}

// Validate checks that PublishOptions values are within acceptable bounds.
// minRetries and maxRetries specify the allowed retry count range.
// Returns an error if any values exceed limits.
func (p *PublishOptions) Validate(minRetries, maxRetries int) error {
	const op = "PublishOptions.Validate"

	if p.RetryCount < minRetries || p.RetryCount > maxRetries {
		return fmt.Errorf("%s: retry_count must be between %d and %d (got: %d)", op, minRetries, maxRetries, p.RetryCount)
	}

	if p.Timeout < 0 {
		return fmt.Errorf("%s: timeout must be non-negative (got: %v)", op, p.Timeout)
	}

	return nil
}

// AdvancedPublisher is an optional interface that publishers can implement
// to provide additional functionality beyond basic publishing.
type AdvancedPublisher interface {
	SocialPublisher

	// PublishWithOptions provides fine-grained control over the publishing process.
	PublishWithOptions(ctx context.Context, content *domain.GeneratedContent, opts *PublishOptions) (*domain.PlatformResult, error)

	// SchedulePost schedules content for future publication (if platform supports it).
	SchedulePost(ctx context.Context, content *domain.GeneratedContent, publishAt time.Time) (*domain.PlatformResult, error)

	// DeletePost removes a previously published post (if platform supports it).
	DeletePost(ctx context.Context, postID string) error
}

// MultiplatformPublisher orchestrates publishing across multiple platforms.
// It handles platform selection, parallel publishing, and result aggregation.
type MultiplatformPublisher interface {
	// PublishToAll publishes content to all configured platforms.
	// Each platform receives content optimized specifically for that platform.
	// Returns aggregated results from all publishing attempts.
	PublishToAll(ctx context.Context, contents map[string]*domain.GeneratedContent) (*domain.PublishResult, error)

	// PublishToSelected publishes to only the specified platforms.
	// Useful when users want to target specific platforms or when some platforms are unavailable.
	PublishToSelected(ctx context.Context, platforms []string, contents map[string]*domain.GeneratedContent) (*domain.PublishResult, error)

	// GetAvailablePlatforms returns a list of platforms that are configured and ready to use.
	GetAvailablePlatforms(ctx context.Context) ([]string, error)

	// TestAllConnections verifies connectivity to all configured platforms.
	// Returns a map of platform -> error (nil if connection successful).
	TestAllConnections(ctx context.Context) (map[string]error, error)
}
