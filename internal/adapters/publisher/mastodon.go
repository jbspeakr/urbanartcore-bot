package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// MastodonAdapter implements the SocialPublisher interface for Mastodon-compatible instances.
// It handles OAuth authentication via access tokens and publishes content with 500-character limit.
// Uses pure REST/HTTP calls to the Mastodon API (no external libraries).
type MastodonAdapter struct {
	server      string       // Server URL (e.g., https://mastodon.social)
	accessToken string       // OAuth access token
	httpClient  *http.Client // HTTP client with timeout
}

// NewMastodonAdapter creates a new MastodonAdapter instance.
// It validates the configuration, initializes the HTTP client, and tests the connection.
func NewMastodonAdapter(server, accessToken string) (*MastodonAdapter, error) {
	const op = "NewMastodonAdapter"

	// Validate server URL
	if server == "" {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("server URL cannot be empty"),
			Retryable: false,
		}
	}

	// Parse and validate server URL
	parsedURL, err := url.Parse(server)
	if err != nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("invalid server URL: %w", err),
			Retryable: false,
		}
	}

	// Enforce HTTPS
	if parsedURL.Scheme != "https" {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("server URL must use HTTPS"),
			Retryable: false,
		}
	}

	// Validate access token
	if accessToken == "" {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("access token cannot be empty"),
			Retryable: false,
		}
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	adapter := &MastodonAdapter{
		server:      server,
		accessToken: accessToken,
		httpClient:  httpClient,
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adapter.TestConnection(ctx); err != nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("connection test failed: %w", err),
			Retryable: false,
		}
	}

	slog.Default().Info("Mastodon adapter initialized",
		slog.String("operation", op),
		slog.String("platform", domain.PlatformMastodon),
		slog.String("server", server),
	)

	return adapter, nil
}

// Platform returns the platform name.
func (a *MastodonAdapter) Platform() string {
	if a == nil {
		return ""
	}
	return domain.PlatformMastodon
}

// GetCharacterLimit returns the maximum character count for Mastodon.
func (a *MastodonAdapter) GetCharacterLimit() int {
	if a == nil {
		return 0
	}
	return domain.MastodonCharacterLimit
}

// SupportsHashtags returns whether Mastodon supports hashtag functionality.
func (a *MastodonAdapter) SupportsHashtags() bool {
	return a != nil
}

// SupportsMedia returns whether Mastodon supports media attachments.
func (a *MastodonAdapter) SupportsMedia() bool {
	return a != nil
}

// ValidateContent checks if content meets Mastodon-specific requirements.
// This includes character limits (500 runes), platform matching, and text validation.
func (a *MastodonAdapter) ValidateContent(ctx context.Context, content *domain.GeneratedContent) error {
	const op = "ValidateContent"

	// Nil safety checks
	if a == nil {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	if content == nil {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("content is nil"),
			Retryable: false,
		}
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       ctx.Err(),
			Retryable: false,
		}
	default:
	}

	// Validate platform matches
	if content.Platform() != domain.PlatformMastodon {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("platform mismatch: expected %s, got %s", domain.PlatformMastodon, content.Platform()),
			Retryable: false,
		}
	}

	// Validate text not empty
	text := content.Text()
	if text == "" {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("text cannot be empty"),
			Retryable: false,
		}
	}

	// Validate character count (use rune count for Mastodon)
	runeCount := len([]rune(text))
	if runeCount > domain.MastodonCharacterLimit {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("%w: %d characters, limit is %d", ports.ErrContentTooLong, runeCount, domain.MastodonCharacterLimit),
			Retryable: false,
		}
	}

	// Validate hashtags format
	for _, hashtag := range content.Hashtags() {
		if hashtag != "" && !strings.HasPrefix(hashtag, "#") {
			return ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("invalid hashtag format: %q (must start with #)", hashtag),
				Retryable: false,
			}
		}
	}

	return nil
}

// TestConnection verifies that the adapter can connect to the Mastodon instance.
// Makes a REST call to /api/v1/accounts/verify_credentials to validate the access token.
func (a *MastodonAdapter) TestConnection(ctx context.Context) error {
	const op = "TestConnection"

	// Nil safety check
	if a == nil {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       ctx.Err(),
			Retryable: false,
		}
	default:
	}

	startTime := time.Now()

	// Call verify_credentials endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.server+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to create request: %w", err),
			Retryable: false,
		}
	}

	req.Header.Set("Authorization", "Bearer "+a.accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		if isNetworkError(err) {
			return ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("%w: %v", ports.ErrNetworkTimeout, err),
				Retryable: true,
			}
		}
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("connection test failed: %w", err),
			Retryable: true,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("%w: invalid access token (status %d)", ports.ErrAuthenticationFailed, resp.StatusCode),
			Retryable: false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			bodyBytes = []byte("(failed to read body)")
		}
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("connection test failed with status %d: %s", resp.StatusCode, string(bodyBytes)),
			Retryable: resp.StatusCode >= 500,
		}
	}

	// Parse response to get account ID
	var accountResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&accountResp); err != nil {
		return ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to decode account response: %w", err),
			Retryable: true,
		}
	}

	responseTime := time.Since(startTime)

	slog.Default().Info("Mastodon connection test successful",
		slog.String("operation", op),
		slog.String("platform", domain.PlatformMastodon),
		slog.String("account_id", accountResp.ID),
		slog.Duration("response_time", responseTime),
	)

	return nil
}

// Publish sends content to the Mastodon platform.
func (a *MastodonAdapter) Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error) {
	const op = "Publish"

	// Nil safety check
	if a == nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	if content == nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       errors.New("content is nil"),
			Retryable: false,
		}
	}

	// Validate content first
	if err := a.ValidateContent(ctx, content); err != nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("content validation failed: %w", err),
			Retryable: false,
		}
	}

	// Create platform result with pending status
	result, err := domain.NewPlatformResult(domain.PlatformMastodon, domain.StatusPending, content)
	if err != nil {
		return nil, ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to create platform result: %w", err),
			Retryable: false,
		}
	}

	startTime := time.Now()

	// Format toot content
	tootText := formatTootContent(content)

	// Build POST request body
	postData := map[string]string{
		"status":     tootText,
		"visibility": "public",
	}

	body, err := json.Marshal(postData)
	if err != nil {
		pubErr := ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to marshal post data: %w", err),
			Retryable: false,
		}
		result.SetFailure(pubErr, 0)
		return result, pubErr
	}

	// Create POST request to /api/v1/statuses
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+"/api/v1/statuses", bytes.NewReader(body))
	if err != nil {
		pubErr := ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to create request: %w", err),
			Retryable: false,
		}
		result.SetFailure(pubErr, 0)
		return result, pubErr
	}

	req.Header.Set("Authorization", "Bearer "+a.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		responseTime := time.Since(startTime)
		var pubErr error
		if isNetworkError(err) {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("%w: %v", ports.ErrNetworkTimeout, err),
				Retryable: true,
			}
		} else {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("publish failed: %w", err),
				Retryable: true,
			}
		}
		result.SetFailure(pubErr, responseTime)
		return result, pubErr
	}
	defer func() { _ = resp.Body.Close() }()

	responseTime := time.Since(startTime)

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			bodyBytes = []byte("(failed to read body)")
		}
		errStr := string(bodyBytes)

		var pubErr error
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("%w: status %d", ports.ErrAuthenticationFailed, resp.StatusCode),
				Retryable: false,
			}
		} else if resp.StatusCode == http.StatusTooManyRequests {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("%w: status %d", ports.ErrRateLimitExceeded, resp.StatusCode),
				Retryable: true,
			}
		} else if resp.StatusCode == http.StatusUnprocessableEntity && isDuplicateError(errors.New(errStr)) {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("%w: status %d", ports.ErrDuplicatePost, resp.StatusCode),
				Retryable: false,
			}
		} else {
			pubErr = ports.PublisherError{
				Op:        op,
				Platform:  domain.PlatformMastodon,
				Err:       fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, errStr),
				Retryable: resp.StatusCode >= 500,
			}
		}

		result.SetFailure(pubErr, responseTime)
		slog.Default().Error("Mastodon publish failed",
			slog.String("operation", op),
			slog.String("platform", domain.PlatformMastodon),
			slog.String("error", pubErr.Error()),
			slog.Duration("response_time", responseTime),
		)
		return result, pubErr
	}

	// Parse response to extract status URL
	var statusResp struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		pubErr := ports.PublisherError{
			Op:        op,
			Platform:  domain.PlatformMastodon,
			Err:       fmt.Errorf("failed to decode status response: %w", err),
			Retryable: true,
		}
		result.SetFailure(pubErr, responseTime)
		return result, pubErr
	}

	// Success - extract post URL
	postURL := statusResp.URL

	result.SetSuccess(postURL, responseTime)

	slog.Default().Info("Mastodon publish successful",
		slog.String("operation", op),
		slog.String("platform", domain.PlatformMastodon),
		slog.String("status_id", statusResp.ID),
		slog.String("url", postURL),
		slog.Duration("response_time", responseTime),
	)

	return result, nil
}

// formatTootContent formats the content for Mastodon by combining text and hashtags.
// Hashtags are appended with double newlines for visual separation.
func formatTootContent(content *domain.GeneratedContent) string {
	if content == nil {
		return ""
	}

	text := content.Text()
	hashtags := content.Hashtags()

	if len(hashtags) == 0 {
		return text
	}

	// Join hashtags with spaces
	hashtagStr := strings.Join(hashtags, " ")

	// Combine text and hashtags with double newline
	return text + "\n\n" + hashtagStr
}

// isAuthError checks if an error is an authentication error (401, 403).
func isAuthError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common auth error indicators
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "Unauthorized") ||
		strings.Contains(errStr, "Forbidden") ||
		strings.Contains(errStr, "invalid_token") ||
		strings.Contains(errStr, "authentication")
}

// isNetworkError checks if an error is a network-related error.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common network error indicators
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dial") ||
		errors.Is(err, context.DeadlineExceeded)
}

// isRateLimitError checks if an error is a rate limit error (429).
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common rate limit error indicators
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests")
}

// isDuplicateError checks if an error is a duplicate post error.
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common duplicate error indicators
	return strings.Contains(errStr, "duplicate") ||
		strings.Contains(errStr, "already exists") ||
		strings.Contains(errStr, "already posted")
}
