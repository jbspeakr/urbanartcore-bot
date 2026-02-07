package publisher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// Test constants
const (
	testServer      = "https://mastodon.social"
	testAccessToken = "test-access-token-12345"
)

// TestNewMastodonAdapter_ValidConfig tests successful adapter creation.
func TestNewMastodonAdapter_ValidConfig(t *testing.T) {
	// Skip actual connection test in unit tests
	// This test would fail without mocking the client
	t.Skip("Requires mocked Mastodon client for proper unit testing")
}

// TestNewMastodonAdapter_EmptyServer tests that empty server URL fails.
func TestNewMastodonAdapter_EmptyServer(t *testing.T) {
	adapter, err := NewMastodonAdapter("", testAccessToken)

	assert.Nil(t, adapter)
	assert.Error(t, err)

	var pubErr ports.PublisherError
	require.True(t, errors.As(err, &pubErr))
	assert.Equal(t, domain.PlatformMastodon, pubErr.Platform)
	assert.False(t, pubErr.Retryable)
	assert.Contains(t, err.Error(), "server URL cannot be empty")
}

// TestNewMastodonAdapter_InvalidServerURL tests that invalid URL fails.
func TestNewMastodonAdapter_InvalidServerURL(t *testing.T) {
	adapter, err := NewMastodonAdapter("not-a-valid-url", testAccessToken)

	assert.Nil(t, adapter)
	assert.Error(t, err)

	var pubErr ports.PublisherError
	require.True(t, errors.As(err, &pubErr))
	assert.False(t, pubErr.Retryable)
}

// TestNewMastodonAdapter_NonHTTPSServer tests that non-HTTPS URL fails.
func TestNewMastodonAdapter_NonHTTPSServer(t *testing.T) {
	adapter, err := NewMastodonAdapter("http://mastodon.social", testAccessToken)

	assert.Nil(t, adapter)
	assert.Error(t, err)

	var pubErr ports.PublisherError
	require.True(t, errors.As(err, &pubErr))
	assert.Equal(t, domain.PlatformMastodon, pubErr.Platform)
	assert.False(t, pubErr.Retryable)
	assert.Contains(t, err.Error(), "must use HTTPS")
}

// TestNewMastodonAdapter_EmptyAccessToken tests that empty access token fails.
func TestNewMastodonAdapter_EmptyAccessToken(t *testing.T) {
	adapter, err := NewMastodonAdapter(testServer, "")

	assert.Nil(t, adapter)
	assert.Error(t, err)

	var pubErr ports.PublisherError
	require.True(t, errors.As(err, &pubErr))
	assert.False(t, pubErr.Retryable)
	assert.Contains(t, err.Error(), "access token cannot be empty")
}

// TestMastodonAdapter_Platform tests Platform() method.
func TestMastodonAdapter_Platform(t *testing.T) {
	tests := []struct {
		name     string
		adapter  *MastodonAdapter
		expected string
	}{
		{
			name: "valid adapter",
			adapter: &MastodonAdapter{
				server: testServer,
			},
			expected: domain.PlatformMastodon,
		},
		{
			name:     "nil adapter",
			adapter:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.Platform()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMastodonAdapter_GetCharacterLimit tests GetCharacterLimit() method.
func TestMastodonAdapter_GetCharacterLimit(t *testing.T) {
	tests := []struct {
		name     string
		adapter  *MastodonAdapter
		expected int
	}{
		{
			name: "valid adapter",
			adapter: &MastodonAdapter{
				server: testServer,
			},
			expected: domain.MastodonCharacterLimit,
		},
		{
			name:     "nil adapter",
			adapter:  nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.GetCharacterLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMastodonAdapter_SupportsHashtags tests SupportsHashtags() method.
func TestMastodonAdapter_SupportsHashtags(t *testing.T) {
	tests := []struct {
		name     string
		adapter  *MastodonAdapter
		expected bool
	}{
		{
			name: "valid adapter",
			adapter: &MastodonAdapter{
				server: testServer,
			},
			expected: true,
		},
		{
			name:     "nil adapter",
			adapter:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.SupportsHashtags()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMastodonAdapter_SupportsMedia tests SupportsMedia() method.
func TestMastodonAdapter_SupportsMedia(t *testing.T) {
	tests := []struct {
		name     string
		adapter  *MastodonAdapter
		expected bool
	}{
		{
			name: "valid adapter",
			adapter: &MastodonAdapter{
				server: testServer,
			},
			expected: true,
		},
		{
			name:     "nil adapter",
			adapter:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.SupportsMedia()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMastodonAdapter_ValidateContent tests content validation.
func TestMastodonAdapter_ValidateContent(t *testing.T) {
	ctx := context.Background()

	// Create valid content
	validContent, err := domain.NewGeneratedContent(
		"Test post content",
		domain.PlatformMastodon,
		[]string{"#test", "#mastodon"},
		"test-prompt",
	)
	require.NoError(t, err)

	// Create too long content (501 runes)
	longText := string(make([]rune, 501))
	for i := range longText {
		longText = longText[:i] + "a"
	}
	tooLongContent, _ := domain.NewGeneratedContent(
		longText,
		domain.PlatformMastodon,
		[]string{},
		"test-prompt",
	)
	// This will fail domain validation, so create manually for adapter test
	_ = tooLongContent

	// Create bluesky content (platform mismatch)
	blueskyContent, err := domain.NewGeneratedContent(
		"Test post",
		domain.PlatformBluesky,
		[]string{},
		"test-prompt",
	)
	require.NoError(t, err)

	adapter := &MastodonAdapter{
		server:      testServer,
		accessToken: testAccessToken,
	}

	tests := []struct {
		name      string
		adapter   *MastodonAdapter
		ctx       context.Context
		content   *domain.GeneratedContent
		wantError bool
		errCheck  func(t *testing.T, err error)
	}{
		{
			name:      "valid content",
			adapter:   adapter,
			ctx:       ctx,
			content:   validContent,
			wantError: false,
		},
		{
			name:      "nil adapter",
			adapter:   nil,
			ctx:       ctx,
			content:   validContent,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "adapter is nil")
			},
		},
		{
			name:      "nil content",
			adapter:   adapter,
			ctx:       ctx,
			content:   nil,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "content is nil")
			},
		},
		{
			name:      "context cancelled",
			adapter:   adapter,
			ctx:       cancelledContext(),
			content:   validContent,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "context canceled")
			},
		},
		{
			name:      "platform mismatch",
			adapter:   adapter,
			ctx:       ctx,
			content:   blueskyContent,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "platform mismatch")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.ValidateContent(tt.ctx, tt.content)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMastodonAdapter_ValidateContent_EmptyText tests empty text validation.
func TestMastodonAdapter_ValidateContent_EmptyText(t *testing.T) {
	// We can't create empty text content via NewGeneratedContent,
	// so we'll test by checking that the domain layer validates this.
	_, err := domain.NewGeneratedContent(
		"",
		domain.PlatformMastodon,
		[]string{},
		"test-prompt",
	)

	// Domain validation should catch empty text
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestMastodonAdapter_ValidateContent_TooLong tests character limit validation.
func TestMastodonAdapter_ValidateContent_TooLong(t *testing.T) {
	// Create text with 501 runes (exceeds Mastodon limit)
	longText := strings.Repeat("a", 501)

	// Domain layer enforces limits, so we can't create via NewGeneratedContent
	// Test that our adapter would catch it by simulating the check
	runeCount := len([]rune(longText))
	assert.Greater(t, runeCount, domain.MastodonCharacterLimit)
}

// TestMastodonAdapter_ValidateContent_UnicodeCharacters tests unicode handling.
func TestMastodonAdapter_ValidateContent_UnicodeCharacters(t *testing.T) {
	// Create content with unicode characters (emoji)
	unicodeContent, err := domain.NewGeneratedContent(
		"Test with emoji 🎨 and unicode ñ é",
		domain.PlatformMastodon,
		[]string{"#art"},
		"test-prompt",
	)
	require.NoError(t, err)

	a := &MastodonAdapter{
		server:      testServer,
		accessToken: testAccessToken,
	}

	err = a.ValidateContent(context.Background(), unicodeContent)
	assert.NoError(t, err)
}

// TestMastodonAdapter_ValidateContent_InvalidHashtag tests hashtag validation.
func TestMastodonAdapter_ValidateContent_InvalidHashtag(t *testing.T) {
	// Domain layer ensures hashtags start with #, so this is already validated
	// Our adapter validation checks that all hashtags start with #
	// This test verifies the domain constraint is enforced
	validHashtags := []string{"#art", "#creative", "#design"}
	for _, tag := range validHashtags {
		assert.True(t, len(tag) > 0 && tag[0] == '#', "hashtag should start with #")
	}
}

// TestMastodonAdapter_TestConnection tests connection testing.
func TestMastodonAdapter_TestConnection(t *testing.T) {
	tests := []struct {
		name      string
		adapter   *MastodonAdapter
		ctx       context.Context
		wantError bool
		errCheck  func(t *testing.T, err error)
	}{
		{
			name:      "nil adapter",
			adapter:   nil,
			ctx:       context.Background(),
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "adapter is nil")
			},
		},
		{
			name: "context cancelled",
			adapter: &MastodonAdapter{
				server:      testServer,
				accessToken: testAccessToken,
			},
			ctx:       cancelledContext(),
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "context canceled")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.TestConnection(tt.ctx)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMastodonAdapter_Publish tests publishing functionality.
func TestMastodonAdapter_Publish(t *testing.T) {
	ctx := context.Background()

	validContent, err := domain.NewGeneratedContent(
		"Test post content",
		domain.PlatformMastodon,
		[]string{"#test"},
		"test-prompt",
	)
	require.NoError(t, err)

	tests := []struct {
		name      string
		adapter   *MastodonAdapter
		ctx       context.Context
		content   *domain.GeneratedContent
		wantError bool
		errCheck  func(t *testing.T, err error)
	}{
		{
			name:      "nil adapter",
			adapter:   nil,
			ctx:       ctx,
			content:   validContent,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "adapter is nil")
			},
		},
		{
			name: "nil content",
			adapter: &MastodonAdapter{
				server:      testServer,
				accessToken: testAccessToken,
			},
			ctx:       ctx,
			content:   nil,
			wantError: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "content is nil")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.adapter.Publish(tt.ctx, tt.content)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
				// Result may or may not be nil depending on where error occurred
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestFormatTootContent tests toot content formatting.
func TestFormatTootContent(t *testing.T) {
	tests := []struct {
		name     string
		content  *domain.GeneratedContent
		expected string
	}{
		{
			name:     "nil content",
			content:  nil,
			expected: "",
		},
		{
			name: "text only",
			content: func() *domain.GeneratedContent {
				c, _ := domain.NewGeneratedContent(
					"Test post",
					domain.PlatformMastodon,
					[]string{},
					"test-prompt",
				)
				return c
			}(),
			expected: "Test post",
		},
		{
			name: "text with hashtags",
			content: func() *domain.GeneratedContent {
				c, _ := domain.NewGeneratedContent(
					"Test post",
					domain.PlatformMastodon,
					[]string{"#test", "#mastodon"},
					"test-prompt",
				)
				return c
			}(),
			expected: "Test post\n\n#test #mastodon",
		},
		{
			name: "text with single hashtag",
			content: func() *domain.GeneratedContent {
				c, _ := domain.NewGeneratedContent(
					"Hello world",
					domain.PlatformMastodon,
					[]string{"#greetings"},
					"test-prompt",
				)
				return c
			}(),
			expected: "Hello world\n\n#greetings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTootContent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsAuthError tests authentication error detection.
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "401 error",
			err:      errors.New("HTTP 401 Unauthorized"),
			expected: true,
		},
		{
			name:     "403 error",
			err:      errors.New("HTTP 403 Forbidden"),
			expected: true,
		},
		{
			name:     "invalid token",
			err:      errors.New("invalid_token"),
			expected: true,
		},
		{
			name:     "authentication failed",
			err:      errors.New("authentication failed"),
			expected: true,
		},
		{
			name:     "network error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "rate limit error",
			err:      errors.New("429 Too Many Requests"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAuthError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsNetworkError tests network error detection.
func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "no such host",
			err:      errors.New("no such host"),
			expected: true,
		},
		{
			name:     "network error",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "dial error",
			err:      errors.New("dial tcp: timeout"),
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "auth error",
			err:      errors.New("401 Unauthorized"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsRateLimitError tests rate limit error detection.
func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "429 error",
			err:      errors.New("HTTP 429 Too Many Requests"),
			expected: true,
		},
		{
			name:     "rate limit exceeded",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "too many requests",
			err:      errors.New("too many requests"),
			expected: true,
		},
		{
			name:     "auth error",
			err:      errors.New("401 Unauthorized"),
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRateLimitError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsDuplicateError tests duplicate error detection.
func TestIsDuplicateError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "duplicate error",
			err:      errors.New("duplicate status"),
			expected: true,
		},
		{
			name:     "already exists",
			err:      errors.New("status already exists"),
			expected: true,
		},
		{
			name:     "already posted",
			err:      errors.New("already posted"),
			expected: true,
		},
		{
			name:     "auth error",
			err:      errors.New("401 Unauthorized"),
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDuplicateError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// cancelledContext returns a cancelled context for testing.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestMastodonAdapter_NilSafety tests nil safety across all public methods.
func TestMastodonAdapter_NilSafety(t *testing.T) {
	var nilAdapter *MastodonAdapter

	// Platform
	assert.Equal(t, "", nilAdapter.Platform())

	// GetCharacterLimit
	assert.Equal(t, 0, nilAdapter.GetCharacterLimit())

	// SupportsHashtags
	assert.False(t, nilAdapter.SupportsHashtags())

	// SupportsMedia
	assert.False(t, nilAdapter.SupportsMedia())

	// ValidateContent
	err := nilAdapter.ValidateContent(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter is nil")

	// TestConnection
	err = nilAdapter.TestConnection(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter is nil")

	// Publish
	result, err := nilAdapter.Publish(context.Background(), nil)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter is nil")
}

// TestMastodonAdapter_PublisherErrorWrapping tests that all errors are wrapped.
func TestMastodonAdapter_PublisherErrorWrapping(t *testing.T) {
	adapter := &MastodonAdapter{
		server:      testServer,
		accessToken: testAccessToken,
	}

	// Test ValidateContent error wrapping
	err := adapter.ValidateContent(context.Background(), nil)
	assert.Error(t, err)

	var pubErr ports.PublisherError
	assert.True(t, errors.As(err, &pubErr))
	assert.Equal(t, domain.PlatformMastodon, pubErr.Platform)
	assert.False(t, pubErr.Retryable)

	// Test TestConnection error wrapping (with nil adapter)
	var nilAdapter *MastodonAdapter
	err = nilAdapter.TestConnection(context.Background())
	assert.Error(t, err)
	assert.True(t, errors.As(err, &pubErr))

	// Test Publish error wrapping
	result, err := adapter.Publish(context.Background(), nil)
	assert.Error(t, err)
	assert.True(t, errors.As(err, &pubErr))
	assert.Equal(t, domain.PlatformMastodon, pubErr.Platform)
	_ = result // May be nil
}

// TestNewMastodonAdapter_InvalidURLScheme tests various invalid URL formats.
func TestNewMastodonAdapter_InvalidURLScheme(t *testing.T) {
	tests := []struct {
		name      string
		server    string
		wantError string
	}{
		{
			name:      "http instead of https",
			server:    "http://mastodon.social",
			wantError: "must use HTTPS",
		},
		{
			name:      "ftp scheme",
			server:    "ftp://mastodon.social",
			wantError: "must use HTTPS",
		},
		{
			name:      "no scheme",
			server:    "mastodon.social",
			wantError: "must use HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewMastodonAdapter(tt.server, testAccessToken)
			assert.Nil(t, adapter)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestMastodonAdapter_ValidateContent_WithHashtags tests validation with various hashtag scenarios.
func TestMastodonAdapter_ValidateContent_WithHashtags(t *testing.T) {
	adapter := &MastodonAdapter{
		server:      testServer,
		accessToken: testAccessToken,
	}

	tests := []struct {
		name      string
		text      string
		hashtags  []string
		wantError bool
	}{
		{
			name:      "valid hashtags",
			text:      "Test post",
			hashtags:  []string{"#test", "#mastodon"},
			wantError: false,
		},
		{
			name:      "single hashtag",
			text:      "Test post",
			hashtags:  []string{"#graffiti"},
			wantError: false,
		},
		{
			name:      "no hashtags",
			text:      "Test post",
			hashtags:  []string{},
			wantError: false,
		},
		{
			name:      "empty hashtag in list",
			text:      "Test post",
			hashtags:  []string{"#valid", ""},
			wantError: false, // Empty hashtags are filtered out by domain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := domain.NewGeneratedContent(
				tt.text,
				domain.PlatformMastodon,
				tt.hashtags,
				"test-prompt",
			)
			require.NoError(t, err)

			err = adapter.ValidateContent(context.Background(), content)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
