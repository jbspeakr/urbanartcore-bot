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
		"https://www.example.com/test",
	)

	require.NoError(t, err)
	assert.NotNil(t, content)
	assert.Equal(t, "Check out this amazing street art piece!", content.Text())
	assert.Equal(t, PlatformBluesky, content.Platform())
	assert.Equal(t, []string{"#StreetArt", "#Graffiti", "#UrbanArt"}, content.Hashtags())
	assert.Equal(t, prompt, content.PromptUsed())
	assert.Equal(t, "https://www.example.com/test", content.URI())
	assert.True(t, content.IsWithinLimit())
}

func TestNewGeneratedContent_InvalidPlatform(t *testing.T) {
	content, err := NewGeneratedContent("Text", "invalid", []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidPlatform))
}

func TestNewGeneratedContent_EmptyText(t *testing.T) {
	content, err := NewGeneratedContent("", PlatformBluesky, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrEmptyGeneratedText))
}

func TestNewGeneratedContent_TextTooLong(t *testing.T) {
	// Create text longer than Bluesky limit
	longText := strings.Repeat("A", BlueskyCharacterLimit+1)

	content, err := NewGeneratedContent(longText, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestNewGeneratedContent_EmptyURI(t *testing.T) {
	content, err := NewGeneratedContent("Valid text", PlatformBluesky, []string{}, "", "")

	assert.Nil(t, content)
	assert.Error(t, err)
	var domainErr DomainError
	assert.True(t, errors.As(err, &domainErr))
	assert.Contains(t, err.Error(), "uri cannot be empty")
}

func TestGeneratedContent_URI(t *testing.T) {
	testURI := "https://www.urbanartcore.com/test-post/"
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", testURI)

	require.NoError(t, err)
	assert.Equal(t, testURI, content.URI())
}

func TestNewGeneratedContent_HashtagsFormatting(t *testing.T) {
	hashtags := []string{
		"StreetArt",    // No #
		"#Graffiti",    // Already has #
		"  UrbanArt  ", // Whitespace
		"",             // Empty
		"  ",           // Whitespace only
	}

	content, err := NewGeneratedContent("Text", PlatformBluesky, hashtags, "", "https://www.example.com/test")

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
			content, err := NewGeneratedContent(text, tt.platform, []string{}, "", "https://www.example.com/test")

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
	content, err := NewGeneratedContent("Hello, World!", PlatformBluesky, []string{}, "", "https://www.example.com/test")
	require.NoError(t, err)

	assert.Equal(t, 13, content.CharacterCount())
}

func TestNewPlatformResult_ValidInput(t *testing.T) {
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", "https://www.example.com/test")
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

func TestPlatformResult_SetSkipped(t *testing.T) {
	result, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)

	skipReason := "dry-run-bluesky"
	responseTime := 100 * time.Millisecond

	result.SetSkipped(skipReason, responseTime)

	assert.Equal(t, StatusSkipped, result.Status())
	assert.Equal(t, skipReason, result.SkipReason())
	assert.Equal(t, responseTime, result.ResponseTime())
	// Ensure publishedURL is not used for skip reason
	assert.Empty(t, result.PublishedURL())
}

func TestPlatformResult_IncrementRetry(t *testing.T) {
	result, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, result.RetryCount())

	// Increment with max retries = 10
	maxRetries := 10
	ok := result.IncrementRetry(maxRetries)
	assert.True(t, ok)
	assert.Equal(t, 1, result.RetryCount())

	ok = result.IncrementRetry(maxRetries)
	assert.True(t, ok)
	assert.Equal(t, 2, result.RetryCount())

	// Test bounds enforcement
	for i := 2; i < maxRetries; i++ {
		ok := result.IncrementRetry(maxRetries)
		assert.True(t, ok)
	}
	assert.Equal(t, maxRetries, result.RetryCount())

	// Should reject further increments
	ok = result.IncrementRetry(maxRetries)
	assert.False(t, ok)
	assert.Equal(t, maxRetries, result.RetryCount())

	// Test with max retries = 0 (no retries allowed)
	result2, err := NewPlatformResult(PlatformMastodon, StatusPending, nil)
	require.NoError(t, err)
	ok = result2.IncrementRetry(0)
	assert.False(t, ok)
	assert.Equal(t, 0, result2.RetryCount())

	// Test with invalid max retries
	result3, err := NewPlatformResult(PlatformBluesky, StatusPending, nil)
	require.NoError(t, err)
	ok = result3.IncrementRetry(-1)
	assert.False(t, ok)
	assert.Equal(t, 0, result3.RetryCount())
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
	content, err := NewGeneratedContent("Text", PlatformBluesky, []string{}, "", "https://www.example.com/test")
	require.NoError(t, err)

	platformResult, err := NewPlatformResult(PlatformBluesky, StatusSuccess, content)
	require.NoError(t, err)

	err = result.AddPlatformResult(platformResult)
	require.NoError(t, err)

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

func TestGeneratedContent_EmojiBluesky(t *testing.T) {
	// Bluesky counts grapheme clusters, not bytes:
	// 300 emoji are 300 graphemes and 1,200 bytes, which is a valid post
	emoji := strings.Repeat("🎨", BlueskyGraphemeLimit)

	content, err := NewGeneratedContent(emoji, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.True(t, content.IsWithinLimit())
	assert.Equal(t, BlueskyGraphemeLimit, content.CharacterCount())
}

func TestGeneratedContent_BlueskyMultiByteAtLimit(t *testing.T) {
	// 300 graphemes mixing accented letters, CJK and a combining sequence
	text := strings.Repeat("é", 100) + strings.Repeat("街", 100) + strings.Repeat("e\u0301", 100)
	require.Equal(t, 300, CountGraphemes(text))
	require.Greater(t, len(text), 300)

	content, err := NewGeneratedContent(text, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.True(t, content.IsWithinLimit())
}

func TestGeneratedContent_BlueskyOverGraphemeLimit(t *testing.T) {
	emoji := strings.Repeat("🎨", BlueskyGraphemeLimit+1)

	content, err := NewGeneratedContent(emoji, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
	assert.Contains(t, err.Error(), "graphemes")
}

func TestGeneratedContent_BlueskyOverByteLimit(t *testing.T) {
	// A family emoji is one grapheme of 25 bytes: 150 of them stay under
	// 300 graphemes but exceed 3,000 bytes
	family := "👨\u200d👩\u200d👧\u200d👦"
	text := strings.Repeat(family, 150)
	require.Equal(t, 150, CountGraphemes(text))
	require.Greater(t, len(text), BlueskyByteLimit)

	content, err := NewGeneratedContent(text, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
	assert.Contains(t, err.Error(), "bytes")
}

func TestGeneratedContent_BlueskyLimitIncludesHashtags(t *testing.T) {
	// 290 graphemes of text fit, but not with " #StreetArt #Graffiti" appended
	text := strings.Repeat("A", 290)

	content, err := NewGeneratedContent(text, PlatformBluesky, []string{"StreetArt", "Graffiti"}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestGeneratedContent_BlueskyLimitIgnoresStrippedURL(t *testing.T) {
	// The URL is removed before posting, so it does not count against the limit
	text := strings.Repeat("A", 290) + " https://www.urbanartcore.com/a-very-long-post-slug-that-would-not-fit/"

	content, err := NewGeneratedContent(text, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.Equal(t, 290, content.CharacterCount())
}

func TestGeneratedContent_MastodonURLCountsAs23(t *testing.T) {
	url := "https://www.urbanartcore.com/" + strings.Repeat("x", 100) + "/"
	text := strings.Repeat("A", 450) + " " + url

	content, err := NewGeneratedContent(text, PlatformMastodon, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.Equal(t, 450+1+MastodonURLLength, content.CharacterCount())
}

func TestGeneratedContent_MastodonLimitIncludesHashtags(t *testing.T) {
	text := strings.Repeat("A", 490)

	content, err := NewGeneratedContent(text, PlatformMastodon, []string{"StreetArt", "Graffiti"}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestGeneratedContent_RenderedText(t *testing.T) {
	bsky, err := NewGeneratedContent("Text https://example.com/post", PlatformBluesky, []string{"StreetArt"}, "", "https://example.com/post")
	require.NoError(t, err)
	assert.Equal(t, "Text #StreetArt", bsky.RenderedText())

	masto, err := NewGeneratedContent("Text https://example.com/post", PlatformMastodon, []string{"StreetArt"}, "", "https://example.com/post")
	require.NoError(t, err)
	assert.Equal(t, "Text https://example.com/post\n\n#StreetArt", masto.RenderedText())
}

func TestGeneratedContent_IdempotencyKey(t *testing.T) {
	a, err := NewGeneratedContent("Text one", PlatformMastodon, nil, "", "https://example.com/post")
	require.NoError(t, err)
	b, err := NewGeneratedContent("A different text", PlatformMastodon, nil, "", "https://example.com/post")
	require.NoError(t, err)
	c, err := NewGeneratedContent("Text one", PlatformMastodon, nil, "", "https://example.com/other")
	require.NoError(t, err)

	assert.Len(t, a.IdempotencyKey(), 64)
	assert.Equal(t, a.IdempotencyKey(), b.IdempotencyKey(), "same post, platform and day must give the same key")
	assert.NotEqual(t, a.IdempotencyKey(), c.IdempotencyKey())
	assert.Empty(t, (*GeneratedContent)(nil).IdempotencyKey())
}

func TestGeneratedContent_EmojiMastodon(t *testing.T) {
	// Test case: Mastodon with emoji
	// 250 emoji characters = 250 graphemes ≈ 1000 UTF-8 bytes
	// Mastodon counts characters, so 250 ≤ 500 → should PASS
	emoji := strings.Repeat("🎨", 250)

	content, err := NewGeneratedContent(emoji, PlatformMastodon, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.NotNil(t, content)
	assert.True(t, content.IsWithinLimit())
}

func TestGeneratedContent_MastodonEmojiLimit(t *testing.T) {
	// Test case: Mastodon with emoji over limit
	// 501 emoji characters → 501 > 500 → should FAIL
	emoji := strings.Repeat("🎨", 501)

	content, err := NewGeneratedContent(emoji, PlatformMastodon, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))

	// Verify error message mentions characters
	assert.Contains(t, err.Error(), "characters")
}

func TestGeneratedContent_MastodonPlainTextLimit(t *testing.T) {
	// Test case: Mastodon with plain text over limit
	// 501 ASCII characters → 501 > 500 → should FAIL
	text := strings.Repeat("A", 501)

	content, err := NewGeneratedContent(text, PlatformMastodon, []string{}, "", "https://www.example.com/test")

	assert.Nil(t, content)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrTextTooLong))
}

func TestGeneratedContent_BlueskyPlainTextLimit(t *testing.T) {
	// Test case: Bluesky with plain text at limit
	// 300 ASCII characters = 300 graphemes → should PASS
	text := strings.Repeat("A", 300)

	content, err := NewGeneratedContent(text, PlatformBluesky, []string{}, "", "https://www.example.com/test")

	require.NoError(t, err)
	assert.NotNil(t, content)
	assert.True(t, content.IsWithinLimit())
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
