package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGeneratedContent_ValidInput(t *testing.T) {
	hashtags := []string{"StreetArt", "Graffiti", "UrbanArt"}
	prompt := "Generate content for Bluesky"

	content, err := NewGeneratedContent(
		"Check out this amazing street art piece!",
		PlatformBluesky,
		hashtags,
		prompt,
		150,
		0.025,
	)

	require.NoError(t, err)
	assert.NotNil(t, content)
	assert.Equal(t, "Check out this amazing street art piece!", content.Text())
	assert.Equal(t, PlatformBluesky, content.Platform())
	assert.Equal(t, []string{"#StreetArt", "#Graffiti", "#UrbanArt"}, content.Hashtags())
	assert.Equal(t, prompt, content.PromptUsed())
	assert.Equal(t, 150, content.TokenCount())
	assert.Equal(t, 0.025, content.Cost())
	assert.True(t, content.IsWithinLimit())

	// Check metadata
	metadata := content.Metadata()
	assert.Equal(t, "40", metadata["character_count"]) // "Check out this amazing street art piece!" is 40 chars
	assert.Equal(t, "3", metadata["hashtag_count"])
	assert.Equal(t, "300", metadata["platform_limit"])
}

func TestNewGeneratedContent_InvalidPlatform(t *testing.T) {
	content, err := NewGeneratedContent("Text", "invalid", []string{}, "", 0, 0)

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidPlatform))
}

func TestNewGeneratedContent_EmptyText(t *testing.T) {
	content, err := NewGeneratedContent("", PlatformBluesky, []string{}, "", 0, 0)

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrEmptyGeneratedText))
}

func TestNewGeneratedContent_TextTooLong(t *testing.T) {
	// Create text longer than Bluesky limit
	longText := strings.Repeat("A", BlueskyCharacterLimit+1)

	content, err := NewGeneratedContent(longText, PlatformBluesky, []string{}, "", 0, 0)

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestNewGeneratedContent_NegativeTokens(t *testing.T) {
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", -1, 0)

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNegativeTokens))
}

func TestNewGeneratedContent_NegativeCost(t *testing.T) {
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", 0, -0.01)

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNegativeCost))
}

func TestNewGeneratedContent_HashtagsFormatting(t *testing.T) {
	hashtags := []string{
		"StreetArt",    // No #
		"#Graffiti",    // Already has #
		"  UrbanArt  ", // Whitespace
		"",             // Empty
		"  ",           // Whitespace only
	}

	content, err := NewGeneratedContent("Text", PlatformBluesky, hashtags, "", 0, 0)

	require.NoError(t, err)
	expected := []string{"#StreetArt", "#Graffiti", "#UrbanArt"}
	assert.Equal(t, expected, content.Hashtags())
}

func TestGeneratedContent_IsWithinLimit(t *testing.T) {
	tests := []struct {
		platform string
		textLen  int
		expected bool
	}{
		{PlatformBluesky, BlueskyCharacterLimit, true},
		{PlatformBluesky, BlueskyCharacterLimit + 1, false},
		{PlatformMastodon, MastodonCharacterLimit, true},
		{PlatformMastodon, MastodonCharacterLimit + 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			text := strings.Repeat("A", tt.textLen)
			content, err := NewGeneratedContent(text, tt.platform, []string{}, "", 0, 0)

			if tt.expected {
				require.NoError(t, err)
				assert.True(t, content.IsWithinLimit())
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGeneratedContent_CharacterCount(t *testing.T) {
	content, err := NewGeneratedContent("Hello, World!", PlatformBluesky, []string{}, "", 0, 0)
	require.NoError(t, err)

	assert.Equal(t, 13, content.CharacterCount())
}

func TestNewPlatformResult_ValidInput(t *testing.T) {
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", 0, 0)
	require.NoError(t, err)

	result, err := NewPlatformResult(PlatformBluesky, StatusSuccess, content)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, PlatformBluesky, result.Platform())
	assert.Equal(t, StatusSuccess, result.Status())
	assert.True(t, result.IsSuccess())
	assert.Equal(t, content, result.Content())
	assert.Equal(t, 0, result.RetryCount())
}

func TestNewPlatformResult_InvalidPlatform(t *testing.T) {
	result, err := NewPlatformResult("invalid", StatusSuccess, nil)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidPlatform))
}

func TestPlatformResult_SetSuccess(t *testing.T) {
	result, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)

	url := "https://bsky.app/profile/user/post/123"
	responseTime := 500 * time.Millisecond

	result.SetSuccess(url, responseTime)

	assert.Equal(t, StatusSuccess, result.Status())
	assert.Equal(t, url, result.PublishedURL())
	assert.Equal(t, responseTime, result.ResponseTime())
	assert.True(t, result.IsSuccess())
}

func TestPlatformResult_SetFailure(t *testing.T) {
	result, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)

	testError := errors.New("network error")
	responseTime := 2 * time.Second

	result.SetFailure(testError, responseTime)

	assert.Equal(t, StatusFailed, result.Status())
	assert.Equal(t, testError, result.Error())
	assert.Equal(t, responseTime, result.ResponseTime())
	assert.False(t, result.IsSuccess())
}

func TestPlatformResult_IncrementRetry(t *testing.T) {
	result, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, result.RetryCount())

	result.IncrementRetry()
	assert.Equal(t, 1, result.RetryCount())

	result.IncrementRetry()
	assert.Equal(t, 2, result.RetryCount())
}

func TestNewPublishResult_ValidInput(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, post, result.Post())
	assert.True(t, result.IsDryRun())
	assert.Equal(t, 0, len(result.Platforms()))
}

func TestNewPublishResult_NilInputs(t *testing.T) {
	// Nil post
	result, err := NewPublishResult(nil, false)
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestPublishResult_AddPlatformResult(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	// Add platform result with content
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", 100, 0.01)
	require.NoError(t, err)

	platformResult, err := NewPlatformResult(PlatformBluesky, StatusSuccess, content)
	require.NoError(t, err)

	err = result.AddPlatformResult(platformResult)
	require.NoError(t, err)

	// Check totals
	assert.Equal(t, 0.01, result.TotalCost())
	assert.Equal(t, 100, result.TotalTokens())
	assert.Equal(t, platformResult, result.GetPlatformResult(PlatformBluesky))
	assert.Contains(t, result.Platforms(), PlatformBluesky)
}

func TestPublishResult_AddPlatformResult_Nil(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	err = result.AddPlatformResult(nil)
	assert.Error(t, err)
}

func TestPublishResult_IsSuccess(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	// No platforms = not success
	assert.False(t, result.IsSuccess())

	// Add successful platform
	successResult, err := NewPlatformResult(PlatformBluesky, StatusSuccess, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(successResult)
	require.NoError(t, err)

	assert.True(t, result.IsSuccess())

	// Add failed platform
	failedResult, err := NewPlatformResult(PlatformMastodon, StatusFailed, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(failedResult)
	require.NoError(t, err)

	// Now not all successful
	assert.False(t, result.IsSuccess())
}

func TestPublishResult_HasFailures(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	assert.False(t, result.HasFailures())

	// Add successful platform
	successResult, err := NewPlatformResult(PlatformBluesky, StatusSuccess, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(successResult)
	require.NoError(t, err)

	assert.False(t, result.HasFailures())

	// Add failed platform
	failedResult, err := NewPlatformResult(PlatformMastodon, StatusFailed, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(failedResult)
	require.NoError(t, err)

	assert.True(t, result.HasFailures())
}

func TestPublishResult_Counts(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	// Add successful platform
	successResult1, err := NewPlatformResult(PlatformBluesky, StatusSuccess, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(successResult1)
	require.NoError(t, err)

	successResult2, err := NewPlatformResult(PlatformMastodon, StatusSuccess, nil)
	require.NoError(t, err)
	err = result.AddPlatformResult(successResult2)
	require.NoError(t, err)

	// Since we can only have 2 unique valid platforms and we need a failure,
	// let's override one to be failed
	successResult2.SetFailure(errors.New("test error"), 0)

	assert.Equal(t, 1, result.SuccessCount())
	assert.Equal(t, 1, result.FailureCount())
}

func TestPublishResult_Duration(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	// Should return duration since start
	duration1 := result.Duration()
	assert.Greater(t, duration1, time.Duration(0))

	// Complete and check duration
	result.Complete()
	duration2 := result.Duration()

	// Completed duration should be close to first duration
	assert.InDelta(t, duration1.Nanoseconds(), duration2.Nanoseconds(), float64(10*time.Millisecond))
}

func TestPublishResult_String(t *testing.T) {
	post, err := NewPost("https://example.com/test", "2020-01-15")
	require.NoError(t, err)

	result, err := NewPublishResult(post, false)
	require.NoError(t, err)

	str := result.String()
	assert.Contains(t, str, "https://example.com/test")
	assert.Contains(t, str, "success") // No failures = success status in String method
}

func TestIsValidPlatform(t *testing.T) {
	tests := []struct {
		platform string
		expected bool
	}{
		{PlatformBluesky, true},
		{PlatformMastodon, true},
		{PlatformAll, true},
		{"BLUESKY", true},     // Case insensitive
		{"  bluesky  ", true}, // Whitespace
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidPlatform(tt.platform))
		})
	}
}

func TestGetPlatformLimit(t *testing.T) {
	tests := []struct {
		platform string
		expected int
	}{
		{PlatformBluesky, BlueskyCharacterLimit},
		{PlatformMastodon, MastodonCharacterLimit},
		{"invalid", 280}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			assert.Equal(t, tt.expected, getPlatformLimit(tt.platform))
		})
	}
}

func TestValidateTextLength(t *testing.T) {
	// Valid lengths
	assert.NoError(t, validateTextLength(strings.Repeat("A", BlueskyCharacterLimit), PlatformBluesky))
	assert.NoError(t, validateTextLength(strings.Repeat("A", MastodonCharacterLimit), PlatformMastodon))

	// Invalid lengths
	err := validateTextLength(strings.Repeat("A", BlueskyCharacterLimit+1), PlatformBluesky)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))

	err = validateTextLength(strings.Repeat("A", MastodonCharacterLimit+1), PlatformMastodon)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestConstants(t *testing.T) {
	// Verify platform constants
	assert.Equal(t, "bluesky", PlatformBluesky)
	assert.Equal(t, "mastodon", PlatformMastodon)
	assert.Equal(t, "all", PlatformAll)

	// Verify character limits
	assert.Equal(t, 300, BlueskyCharacterLimit)
	assert.Equal(t, 500, MastodonCharacterLimit)

	// Verify status constants
	assert.Equal(t, PublishStatus("success"), StatusSuccess)
	assert.Equal(t, PublishStatus("failed"), StatusFailed)
	assert.Equal(t, PublishStatus("skipped"), StatusSkipped)
	assert.Equal(t, PublishStatus("pending"), StatusPending)
}
