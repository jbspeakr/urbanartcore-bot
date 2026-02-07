package ports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
)

// GeneratorError represents AI content generation errors.
type GeneratorError struct {
	Op        string            // Operation that caused the error
	Model     string            // AI model used
	Err       error             // Underlying error
	Retryable bool              // Whether this error can be retried
	Metadata  map[string]string // Additional error context
}

func (e GeneratorError) Error() string {
	model := e.Model
	if model == "" {
		model = "unknown"
	}

	if e.Op == "" {
		return fmt.Sprintf("generator[%s]: %v", model, e.Err)
	}
	return fmt.Sprintf("generator[%s].%s: %v", model, e.Op, e.Err)
}

func (e GeneratorError) Unwrap() error {
	return e.Err
}

// IsRetryable returns whether this error indicates a retryable condition.
func (e GeneratorError) IsRetryable() bool {
	return e.Retryable
}

// Common generator errors
var (
	ErrAPIKeyInvalid      = errors.New("AI API key is invalid or expired")
	ErrTokenLimitExceeded = errors.New("content exceeds AI model token limit")
	ErrContentFiltered    = errors.New("generated content was filtered by AI safety systems")
	ErrInvalidPrompt      = errors.New("prompt template is invalid or malformed")
	ErrServiceUnavailable = errors.New("AI service is temporarily unavailable")
	ErrModelNotFound      = errors.New("requested AI model is not available")
	ErrInsufficientQuota  = errors.New("AI service quota exhausted")
)

// GenerationRequest contains all information needed to generate platform-specific content.
type GenerationRequest struct {
	Posts       []*domain.Post    `json:"posts"`       // Source posts for anniversary
	Platform    string            `json:"platform"`    // Target platform (bluesky, mastodon)
	DateKey     string            `json:"date_key"`    // MM-DD anniversary date
	Context     map[string]string `json:"context"`     // Additional context for AI
	Temperature float32           `json:"temperature"` // Creativity level (0.0-1.0)
	MaxTokens   int               `json:"max_tokens"`  // Maximum tokens to generate
}

// NewGenerationRequest creates a new request with validation.
func NewGenerationRequest(posts []*domain.Post, platform string, dateKey string) (*GenerationRequest, error) {
	if len(posts) == 0 {
		return nil, fmt.Errorf("generation request must include at least one post")
	}

	if !domain.IsValidPlatform(platform) {
		return nil, fmt.Errorf("invalid platform: %s", platform)
	}

	if dateKey == "" {
		return nil, fmt.Errorf("date key cannot be empty")
	}

	return &GenerationRequest{
		Posts:       posts,
		Platform:    platform,
		DateKey:     dateKey,
		Context:     make(map[string]string),
		Temperature: 0.7, // Balanced creativity
		MaxTokens:   500, // Conservative limit
	}, nil
}

// WithTemperature sets the creativity level for generation.
func (gr *GenerationRequest) WithTemperature(temp float32) *GenerationRequest {
	gr.Temperature = temp
	return gr
}

// WithMaxTokens sets the maximum tokens to generate.
func (gr *GenerationRequest) WithMaxTokens(tokens int) *GenerationRequest {
	gr.MaxTokens = tokens
	return gr
}

// WithContext adds contextual information to help guide generation.
func (gr *GenerationRequest) WithContext(key, value string) *GenerationRequest {
	if gr.Context == nil {
		gr.Context = make(map[string]string)
	}
	gr.Context[key] = value
	return gr
}

// ContentGenerator defines the contract for AI-powered content generation.
// It abstracts the AI service layer to support different providers (Mistral, OpenAI, etc.)
// while maintaining consistent behavior and platform-specific optimization.
type ContentGenerator interface {
	// GenerateContent creates platform-optimized social media content from anniversary posts.
	// The request contains posts, target platform, and generation parameters.
	// Returns generated content with metadata about the generation process.
	GenerateContent(ctx context.Context, req *GenerationRequest) (*domain.GeneratedContent, error)

	// GenerateForAllPlatforms creates content optimized for all supported platforms.
	// This is more efficient than individual calls when publishing to multiple platforms.
	// Returns a map of platform -> generated content.
	GenerateForAllPlatforms(ctx context.Context, posts []*domain.Post, dateKey string) (map[string]*domain.GeneratedContent, error)

	// ValidatePrompt checks if a prompt template is properly formatted and safe to use.
	// Used during application startup to validate prompt templates.
	ValidatePrompt(ctx context.Context, platform string, prompt string) error

	// TestConnection verifies connectivity to the AI service with current credentials.
	// Used during application startup and health checks.
	TestConnection(ctx context.Context) error
}

// GenerationOptions provides fine-grained control over content generation.
type GenerationOptions struct {
	DryRun          bool              `json:"dry_run"`          // Don't consume API quota
	RetryCount      int               `json:"retry_count"`      // Number of retry attempts
	Timeout         time.Duration     `json:"timeout"`          // Generation timeout
	CustomPrompt    string            `json:"custom_prompt"`    // Override default prompt
	PreserveContext bool              `json:"preserve_context"` // Keep conversation context
	Metadata        map[string]string `json:"metadata"`         // Additional generation context
}

// DefaultGenerationOptions returns sensible defaults for content generation.
func DefaultGenerationOptions() *GenerationOptions {
	return &GenerationOptions{
		DryRun:          false,
		RetryCount:      2,
		Timeout:         45 * time.Second,
		PreserveContext: false,
		Metadata:        make(map[string]string),
	}
}

// Validate checks that GenerationOptions values are within acceptable bounds.
// minRetries and maxRetries specify the allowed retry count range.
// Returns an error if any values exceed limits.
func (g *GenerationOptions) Validate(minRetries, maxRetries int) error {
	const op = "GenerationOptions.Validate"

	if g.RetryCount < minRetries || g.RetryCount > maxRetries {
		return fmt.Errorf("%s: retry_count must be between %d and %d (got: %d)", op, minRetries, maxRetries, g.RetryCount)
	}

	if g.Timeout < 0 {
		return fmt.Errorf("%s: timeout must be non-negative (got: %v)", op, g.Timeout)
	}

	return nil
}

// AdvancedGenerator is an optional interface that generators can implement
// to provide additional functionality beyond basic content generation.
type AdvancedGenerator interface {
	ContentGenerator

	// GenerateWithOptions provides fine-grained control over the generation process.
	GenerateWithOptions(ctx context.Context, req *GenerationRequest, opts *GenerationOptions) (*domain.GeneratedContent, error)

	// ImproveContent takes existing content and generates an improved version.
	// Useful for iterative refinement and A/B testing.
	ImproveContent(ctx context.Context, existing *domain.GeneratedContent, feedback string) (*domain.GeneratedContent, error)

	// GenerateVariations creates multiple versions of content for the same request.
	// Useful for A/B testing and finding the most engaging version.
	GenerateVariations(ctx context.Context, req *GenerationRequest, count int) ([]*domain.GeneratedContent, error)
}

// PromptManager handles prompt templates and variable substitution for content generation.
type PromptManager interface {
	// GetPrompt retrieves the prompt template for a specific platform.
	GetPrompt(ctx context.Context, platform string) (string, error)

	// RenderPrompt processes a template with variables from the generation request.
	// Returns the final prompt ready to send to the AI service.
	RenderPrompt(ctx context.Context, platform string, req *GenerationRequest) (string, error)

	// ValidateTemplate checks if a prompt template is syntactically correct.
	ValidateTemplate(ctx context.Context, template string) error

	// ListPrompts returns all available prompt templates by platform.
	ListPrompts(ctx context.Context) (map[string]string, error)
}
