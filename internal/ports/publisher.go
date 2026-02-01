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
	Op        string            // Operation that caused the error
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

	if e.Op == "" {
		return fmt.Sprintf("publisher[%s]: %v", platform, e.Err)
	}
	return fmt.Sprintf("publisher[%s].%s: %v", platform, e.Op, e.Err)
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

	// GetPostStats retrieves engagement metrics for a published post.
	GetPostStats(ctx context.Context, postID string) (map[string]any, error)
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

// PublisherStats provides metrics about publishing operations.
type PublisherStats struct {
	Platform         string        `json:"platform"`
	TotalPublished   int           `json:"total_published"`
	SuccessRate      float64       `json:"success_rate"`      // 0.0 to 1.0
	AverageLatency   time.Duration `json:"average_latency"`   // Publishing response time
	LastSuccess      *time.Time    `json:"last_success"`      // Last successful publish
	LastError        string        `json:"last_error"`        // Most recent error message
	RateLimitStatus  string        `json:"rate_limit_status"` // Current rate limit status
	ConnectionHealth string        `json:"connection_health"` // "healthy", "degraded", "unhealthy"
}

// PublisherStatsProvider is an optional interface that publisher implementations
// can implement to provide operational metrics.
type PublisherStatsProvider interface {
	// GetPublisherStats returns operational metrics for this publisher.
	GetPublisherStats(ctx context.Context) (*PublisherStats, error)
}
