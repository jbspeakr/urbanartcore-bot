package generator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	mistral "github.com/gage-technologies/mistral-go"

	"codeberg.org/brennenstuhl/uac-socializer/internal/app"
	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// MistralAdapter implements the ContentGenerator interface using Mistral AI.
// It provides AI-powered content generation with platform-specific optimization,
// retry logic, and comprehensive error handling.
type MistralAdapter struct {
	config    app.MistralConfig
	client    *mistral.MistralClient
	promptMgr ports.PromptManager
}

// NewMistralAdapter creates a new MistralAdapter with the given configuration.
// It validates the configuration and initializes the Mistral client.
func NewMistralAdapter(config app.MistralConfig, promptMgr ports.PromptManager) (*MistralAdapter, error) {
	const op = "NewMistralAdapter"
	logger := slog.Default()

	// Validate configuration
	if config.APIKey == "" {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: config.Model,
			Err:   ports.ErrAPIKeyInvalid,
		}
	}

	if config.Model == "" {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: "unknown",
			Err:   errors.New("model cannot be empty"),
		}
	}

	if config.MaxTokens <= 0 {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: config.Model,
			Err:   fmt.Errorf("max_tokens must be positive (got: %d)", config.MaxTokens),
		}
	}

	if config.Temperature < 0.0 || config.Temperature > 1.0 {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: config.Model,
			Err:   fmt.Errorf("temperature must be between 0.0 and 1.0 (got: %f)", config.Temperature),
		}
	}

	if promptMgr == nil {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: config.Model,
			Err:   errors.New("prompt manager cannot be nil"),
		}
	}

	// Create Mistral client with custom timeout
	client := mistral.NewMistralClient(
		config.APIKey,
		mistral.Endpoint,
		0, // We handle retries ourselves for better control
		config.Timeout,
	)

	logger.Info("Mistral adapter initialized",
		slog.String("operation", op),
		slog.String("model", config.Model),
		slog.Int("max_tokens", config.MaxTokens),
		slog.Float64("temperature", config.Temperature),
		slog.Int("retry_count", config.RetryCount))

	return &MistralAdapter{
		config:    config,
		client:    client,
		promptMgr: promptMgr,
	}, nil
}

// GenerateContent creates platform-optimized social media content from anniversary posts.
func (m *MistralAdapter) GenerateContent(ctx context.Context, req *ports.GenerationRequest) (*domain.GeneratedContent, error) {
	const op = "GenerateContent"
	logger := slog.Default()

	if req == nil {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: m.config.Model,
			Err:   errors.New("generation request cannot be nil"),
		}
	}

	logger.Info("Generating content",
		slog.String("operation", op),
		slog.String("platform", req.Platform),
		slog.String("date_key", req.DateKey),
		slog.Int("post_count", len(req.Posts)),
		slog.String("model", m.config.Model))

	// Render prompt template
	prompt, err := m.promptMgr.RenderPrompt(ctx, req.Platform, req)
	if err != nil {
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("failed to render prompt: %w", err),
			Retryable: false,
		}
	}

	// Generate content with retry logic
	content, err := m.generateWithRetry(ctx, prompt, req.Platform)
	if err != nil {
		return nil, err // Error already wrapped by generateWithRetry
	}

	logger.Info("Content generated successfully",
		slog.String("operation", op),
		slog.String("platform", req.Platform))

	return content, nil
}

// GenerateForAllPlatforms creates content optimized for all supported platforms.
func (m *MistralAdapter) GenerateForAllPlatforms(ctx context.Context, posts []*domain.Post, dateKey string) (map[string]*domain.GeneratedContent, error) {
	const op = "GenerateForAllPlatforms"
	logger := slog.Default()

	if len(posts) == 0 {
		return nil, ports.GeneratorError{
			Op:    op,
			Model: m.config.Model,
			Err:   errors.New("posts cannot be empty"),
		}
	}

	platforms := []string{domain.PlatformBluesky, domain.PlatformMastodon}
	results := make(map[string]*domain.GeneratedContent)

	logger.Info("Generating content for all platforms",
		slog.String("operation", op),
		slog.String("date_key", dateKey),
		slog.Int("post_count", len(posts)),
		slog.Int("platform_count", len(platforms)))

	for _, platform := range platforms {
		// Create generation request
		req, err := ports.NewGenerationRequest(posts, platform, dateKey)
		if err != nil {
			logger.Error("Failed to create generation request",
				slog.String("operation", op),
				slog.String("platform", platform),
				slog.String("error", err.Error()))
			return nil, ports.GeneratorError{
				Op:        op,
				Model:     m.config.Model,
				Err:       fmt.Errorf("platform %s: %w", platform, err),
				Retryable: false,
			}
		}

		// Apply default settings from config
		req.WithTemperature(float32(m.config.Temperature)).
			WithMaxTokens(m.config.MaxTokens)

		// Generate content for this platform
		content, err := m.GenerateContent(ctx, req)
		if err != nil {
			logger.Error("Failed to generate content for platform",
				slog.String("operation", op),
				slog.String("platform", platform),
				slog.String("error", err.Error()))
			return nil, err
		}

		results[platform] = content
	}

	logger.Info("Content generated for all platforms",
		slog.String("operation", op),
		slog.Int("platforms", len(results)))

	return results, nil
}

// ValidatePrompt checks if a prompt template is properly formatted and safe to use.
func (m *MistralAdapter) ValidatePrompt(ctx context.Context, platform string, prompt string) error {
	const op = "ValidatePrompt"

	if !domain.IsValidPlatform(platform) {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("invalid platform: %s", platform),
			Retryable: false,
		}
	}

	if strings.TrimSpace(prompt) == "" {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       errors.New("prompt cannot be empty"),
			Retryable: false,
		}
	}

	// Use the prompt manager's validation
	if err := m.promptMgr.ValidateTemplate(ctx, prompt); err != nil {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("prompt validation failed: %w", err),
			Retryable: false,
		}
	}

	return nil
}

// TestConnection verifies connectivity to the AI service with current credentials.
func (m *MistralAdapter) TestConnection(ctx context.Context) error {
	const op = "TestConnection"
	logger := slog.Default()

	logger.Info("Testing Mistral API connection",
		slog.String("operation", op),
		slog.String("model", m.config.Model))

	// Create a simple test message
	messages := []mistral.ChatMessage{
		{
			Role:    mistral.RoleUser,
			Content: "Reply with 'OK' if you can read this.",
		},
	}

	params := &mistral.ChatRequestParams{
		Temperature: 0.0,
		MaxTokens:   10,
		SafePrompt:  false,
	}

	// Make a test API call with timeout
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// We need to wrap the call to check for context cancellation
	type result struct {
		resp *mistral.ChatCompletionResponse
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		resp, err := m.client.Chat(m.config.Model, messages, params)
		resultCh <- result{resp: resp, err: err}
	}()

	select {
	case <-testCtx.Done():
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrServiceUnavailable,
			Retryable: true,
		}
	case res := <-resultCh:
		if res.err != nil {
			logger.Error("Connection test failed",
				slog.String("operation", op),
				slog.String("error", res.err.Error()))
			return m.classifyError(op, res.err)
		}

		logger.Info("Connection test successful",
			slog.String("operation", op),
			slog.String("model", m.config.Model))
		return nil
	}
}

// generateWithRetry generates content with exponential backoff retry logic.
func (m *MistralAdapter) generateWithRetry(ctx context.Context, prompt, platform string) (*domain.GeneratedContent, error) {
	const op = "generateWithRetry"
	logger := slog.Default()

	var lastErr error

	for attempt := 0; attempt <= m.config.RetryCount; attempt++ {
		// Check context before attempting
		select {
		case <-ctx.Done():
			return nil, ports.GeneratorError{
				Op:        op,
				Model:     m.config.Model,
				Err:       ctx.Err(),
				Retryable: false,
			}
		default:
		}

		if attempt > 0 {
			// Calculate exponential backoff: 1s * 2^attempt, capped at 30s
			const maxBackoffDelay = 30 * time.Second
			delay := min(
				time.Duration(math.Pow(2, float64(attempt)))*time.Second,
				maxBackoffDelay,
			)
			logger.Warn("Retrying content generation",
				slog.String("operation", op),
				slog.Int("attempt", attempt),
				slog.Int("max_retries", m.config.RetryCount),
				slog.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ports.GeneratorError{
					Op:        op,
					Model:     m.config.Model,
					Err:       ctx.Err(),
					Retryable: false,
				}
			}
		}

		// Attempt generation
		content, err := m.generate(ctx, prompt, platform)

		if err == nil {
			return content, nil
		}

		lastErr = err

		// Check if error is retryable
		var genErr ports.GeneratorError
		if errors.As(err, &genErr) {
			if !genErr.IsRetryable() {
				logger.Error("Non-retryable error encountered",
					slog.String("operation", op),
					slog.String("error", err.Error()))
				return nil, err
			}
		}

		logger.Warn("Generation attempt failed",
			slog.String("operation", op),
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()))
	}

	// All retries exhausted
	logger.Error("All retry attempts exhausted",
		slog.String("operation", op),
		slog.Int("attempts", m.config.RetryCount+1),
		slog.String("error", lastErr.Error()))

	return nil, ports.GeneratorError{
		Op:        op,
		Model:     m.config.Model,
		Err:       fmt.Errorf("exhausted %d retries: %w", m.config.RetryCount, lastErr),
		Retryable: false,
	}
}

// generate performs a single content generation attempt.
func (m *MistralAdapter) generate(ctx context.Context, prompt, platform string) (*domain.GeneratedContent, error) {
	const op = "generate"

	// Prepare messages for Mistral API
	messages := []mistral.ChatMessage{
		{
			Role:    mistral.RoleSystem,
			Content: "You are a content creator for Urban Art Core, specializing in creating engaging social media posts about street art and graffiti culture. Your responses must be valid JSON only.",
		},
		{
			Role:    mistral.RoleUser,
			Content: prompt,
		},
	}

	// Configure generation parameters
	params := &mistral.ChatRequestParams{
		Temperature:    m.config.Temperature,
		MaxTokens:      m.config.MaxTokens,
		SafePrompt:     false,
		ResponseFormat: mistral.ResponseFormatJsonObject,
	}

	// Call Mistral API with context awareness
	type result struct {
		resp *mistral.ChatCompletionResponse
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		resp, err := m.client.Chat(m.config.Model, messages, params)
		resultCh <- result{resp: resp, err: err}
	}()

	var resp *mistral.ChatCompletionResponse
	select {
	case <-ctx.Done():
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ctx.Err(),
			Retryable: false,
		}
	case res := <-resultCh:
		if res.err != nil {
			return nil, m.classifyError(op, res.err)
		}
		resp = res.resp
	}

	// Extract response
	if len(resp.Choices) == 0 {
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       errors.New("no choices in response"),
			Retryable: true,
		}
	}

	choice := resp.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)

	// Parse JSON response
	var apiResponse struct {
		Text           string   `json:"text"`
		Hashtags       []string `json:"hashtags"`
		ByteCount      int      `json:"byte_count"`
		CharacterCount int      `json:"character_count"`
	}

	if err := json.Unmarshal([]byte(content), &apiResponse); err != nil {
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("failed to parse JSON response: %w", err),
			Retryable: true, // AI might fix JSON on retry
		}
	}

	// Validate response fields
	if apiResponse.Text == "" {
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       errors.New("generated text is empty"),
			Retryable: true,
		}
	}

	// Create domain object with validation
	generatedContent, err := domain.NewGeneratedContent(
		apiResponse.Text,
		platform,
		apiResponse.Hashtags,
		prompt,
	)
	if err != nil {
		return nil, ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("content validation failed: %w", err),
			Retryable: true, // AI might generate valid content on retry
		}
	}

	return generatedContent, nil
}

// classifyError classifies API errors into retryable and non-retryable categories.
func (m *MistralAdapter) classifyError(op string, err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	errStrLower := strings.ToLower(errStr)

	// Check for authentication errors (non-retryable)
	if strings.Contains(errStrLower, "unauthorized") ||
		strings.Contains(errStrLower, "invalid api key") ||
		strings.Contains(errStrLower, "401") ||
		strings.Contains(errStrLower, "403") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrAPIKeyInvalid,
			Retryable: false,
		}
	}

	// Check for rate limiting (retryable)
	if strings.Contains(errStrLower, "rate limit") ||
		strings.Contains(errStrLower, "429") ||
		strings.Contains(errStrLower, "too many requests") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       errors.New("rate limit exceeded"),
			Retryable: true,
		}
	}

	// Check for server errors (retryable)
	if strings.Contains(errStrLower, "500") ||
		strings.Contains(errStrLower, "502") ||
		strings.Contains(errStrLower, "503") ||
		strings.Contains(errStrLower, "504") ||
		strings.Contains(errStrLower, "internal server error") ||
		strings.Contains(errStrLower, "service unavailable") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrServiceUnavailable,
			Retryable: true,
		}
	}

	// Check for timeout (retryable)
	if strings.Contains(errStrLower, "timeout") ||
		strings.Contains(errStrLower, "deadline exceeded") ||
		errors.Is(err, context.DeadlineExceeded) {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       errors.New("request timeout"),
			Retryable: true,
		}
	}

	// Check for content filtering (non-retryable)
	if strings.Contains(errStrLower, "content filtered") ||
		strings.Contains(errStrLower, "safety") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrContentFiltered,
			Retryable: false,
		}
	}

	// Check for model not found (non-retryable)
	if strings.Contains(errStrLower, "model not found") ||
		strings.Contains(errStrLower, "invalid model") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrModelNotFound,
			Retryable: false,
		}
	}

	// Check for quota exhausted (non-retryable in short term)
	if strings.Contains(errStrLower, "quota") ||
		strings.Contains(errStrLower, "insufficient") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       ports.ErrInsufficientQuota,
			Retryable: false,
		}
	}

	// Network errors (retryable)
	if strings.Contains(errStrLower, "connection") ||
		strings.Contains(errStrLower, "network") ||
		strings.Contains(errStrLower, "dns") {
		return ports.GeneratorError{
			Op:        op,
			Model:     m.config.Model,
			Err:       fmt.Errorf("network error: %w", err),
			Retryable: true,
		}
	}

	// Default: treat as retryable with original error
	return ports.GeneratorError{
		Op:        op,
		Model:     m.config.Model,
		Err:       err,
		Retryable: true,
	}
}

// Ensure MistralAdapter implements ContentGenerator interface
var _ ports.ContentGenerator = (*MistralAdapter)(nil)

// HTTPStatusError represents an HTTP status error for testing purposes.
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// NewHTTPStatusError creates a new HTTPStatusError for testing.
func NewHTTPStatusError(statusCode int, message string) error {
	return &HTTPStatusError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// GetHTTPStatusCode extracts the HTTP status code from an error if available.
func GetHTTPStatusCode(err error) int {
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}

	// Try to extract from error string
	errStr := err.Error()
	if strings.Contains(errStr, "401") {
		return http.StatusUnauthorized
	}
	if strings.Contains(errStr, "403") {
		return http.StatusForbidden
	}
	if strings.Contains(errStr, "429") {
		return http.StatusTooManyRequests
	}
	if strings.Contains(errStr, "500") {
		return http.StatusInternalServerError
	}
	if strings.Contains(errStr, "502") {
		return http.StatusBadGateway
	}
	if strings.Contains(errStr, "503") {
		return http.StatusServiceUnavailable
	}
	if strings.Contains(errStr, "504") {
		return http.StatusGatewayTimeout
	}

	return 0
}
