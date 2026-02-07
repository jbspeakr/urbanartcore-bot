package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Validation errors
var (
	ErrInvalidHashtagFormat  = errors.New("invalid hashtag format")
	ErrInvalidHashtagCount   = errors.New("invalid hashtag count for platform")
	ErrDuplicateHashtag      = errors.New("duplicate hashtag detected")
	ErrEmptyContent          = errors.New("content text is empty")
	ErrContentTooShort       = errors.New("content is too short")
	ErrInjectionDetected     = errors.New("potential injection pattern detected")
	ErrURINotFound           = errors.New("expected URI not found in content")
	ErrInvalidContentQuality = errors.New("content quality check failed")
)

// Hashtag validation constants
const (
	MinHashtagLength    = 2
	MaxHashtagLength    = 50
	MinContentLength    = 10
	BlueskyMinHashtags  = 1
	BlueskyMaxHashtags  = 3
	MastodonMinHashtags = 2
	MastodonMaxHashtags = 4
)

// Regex patterns for validation
var (
	// Hashtag must be alphanumeric and underscores only (no spaces, no special chars except _)
	hashtagPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

	// Injection patterns to detect (basic set for content safety)
	injectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`<script[^>]*>.*?</script>`), // Script tags
		regexp.MustCompile(`javascript:`),               // Javascript protocol
		regexp.MustCompile(`on\w+\s*=`),                 // Event handlers
		regexp.MustCompile(`<iframe[^>]*>`),             // iframes
		regexp.MustCompile(`eval\s*\(`),                 // eval calls
	}
)

// ValidateHashtags validates hashtags for a specific platform.
// It checks:
// - Hashtag format (alphanumeric and underscores only)
// - Platform-specific count requirements
// - No duplicate hashtags
func ValidateHashtags(hashtags []string, platform string) error {
	const op = "ValidateHashtags"

	// Validate platform
	platform = strings.ToLower(strings.TrimSpace(platform))
	if !IsValidPlatform(platform) {
		return DomainError{Op: op, Err: fmt.Errorf("%w: %q", ErrInvalidPlatform, platform)}
	}

	// Determine platform-specific hashtag count requirements
	var minHashtags, maxHashtags int
	switch platform {
	case PlatformBluesky:
		minHashtags = BlueskyMinHashtags
		maxHashtags = BlueskyMaxHashtags
	case PlatformMastodon:
		minHashtags = MastodonMinHashtags
		maxHashtags = MastodonMaxHashtags
	default:
		// For unknown platforms, use reasonable defaults
		minHashtags = 1
		maxHashtags = 5
	}

	// Check hashtag count
	if len(hashtags) < minHashtags {
		return DomainError{
			Op: op,
			Err: fmt.Errorf("%w: %s requires at least %d hashtags, got %d",
				ErrInvalidHashtagCount, platform, minHashtags, len(hashtags)),
		}
	}

	if len(hashtags) > maxHashtags {
		return DomainError{
			Op: op,
			Err: fmt.Errorf("%w: %s allows maximum %d hashtags, got %d",
				ErrInvalidHashtagCount, platform, maxHashtags, len(hashtags)),
		}
	}

	// Track seen hashtags for duplicate detection (case-insensitive)
	seen := make(map[string]struct{})

	// Validate each hashtag
	for i, tag := range hashtags {
		// Remove leading # if present
		cleanTag := strings.TrimPrefix(tag, "#")
		cleanTag = strings.TrimSpace(cleanTag)

		if cleanTag == "" {
			return DomainError{
				Op:  op,
				Err: fmt.Errorf("%w: hashtag at index %d is empty", ErrInvalidHashtagFormat, i),
			}
		}

		// Check length
		if len(cleanTag) < MinHashtagLength {
			return DomainError{
				Op: op,
				Err: fmt.Errorf("%w: hashtag %q is too short (minimum %d characters)",
					ErrInvalidHashtagFormat, cleanTag, MinHashtagLength),
			}
		}

		if len(cleanTag) > MaxHashtagLength {
			return DomainError{
				Op: op,
				Err: fmt.Errorf("%w: hashtag %q is too long (maximum %d characters)",
					ErrInvalidHashtagFormat, cleanTag, MaxHashtagLength),
			}
		}

		// Check format (alphanumeric and underscores only)
		if !hashtagPattern.MatchString(cleanTag) {
			return DomainError{
				Op: op,
				Err: fmt.Errorf("%w: hashtag %q contains invalid characters (only alphanumeric and underscores allowed)",
					ErrInvalidHashtagFormat, cleanTag),
			}
		}

		// Check for duplicates (case-insensitive)
		lowerTag := strings.ToLower(cleanTag)
		if _, exists := seen[lowerTag]; exists {
			return DomainError{
				Op:  op,
				Err: fmt.Errorf("%w: hashtag %q appears multiple times", ErrDuplicateHashtag, cleanTag),
			}
		}
		seen[lowerTag] = struct{}{}
	}

	return nil
}

// ValidateContentQuality performs quality checks on generated content.
// It checks:
// - Non-empty content
// - Minimum length requirement
// - No obvious injection patterns
func ValidateContentQuality(text string) error {
	const op = "ValidateContentQuality"

	// Check for empty content
	text = strings.TrimSpace(text)
	if text == "" {
		return DomainError{Op: op, Err: ErrEmptyContent}
	}

	// Check minimum length
	if len(text) < MinContentLength {
		return DomainError{
			Op: op,
			Err: fmt.Errorf("%w: content has %d characters, minimum is %d",
				ErrContentTooShort, len(text), MinContentLength),
		}
	}

	// Check for injection patterns
	lowerText := strings.ToLower(text)
	for _, pattern := range injectionPatterns {
		if pattern.MatchString(lowerText) {
			return DomainError{
				Op: op,
				Err: fmt.Errorf("%w: found suspicious pattern matching %q",
					ErrInjectionDetected, pattern.String()),
			}
		}
	}

	// Check for excessive control characters (but allow newlines and tabs)
	controlCharCount := 0
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			controlCharCount++
		}
	}

	// If more than 5% of characters are control characters, flag as suspicious
	if len(text) > 0 && float64(controlCharCount)/float64(len(text)) > 0.05 {
		return DomainError{
			Op: op,
			Err: fmt.Errorf("%w: excessive control characters detected (%d of %d)",
				ErrInvalidContentQuality, controlCharCount, len(text)),
		}
	}

	return nil
}

// ContainsURI checks if the expected URI appears in the generated text.
// This ensures the AI-generated content includes the blog post link.
// The URI check is case-insensitive and allows for both full and partial matches.
func ContainsURI(text string, expectedURI string) error {
	const op = "ContainsURI"

	// Validate inputs
	text = strings.TrimSpace(text)
	expectedURI = strings.TrimSpace(expectedURI)

	if text == "" {
		return DomainError{Op: op, Err: ErrEmptyContent}
	}

	if expectedURI == "" {
		return DomainError{Op: op, Err: errors.New("expected URI cannot be empty")}
	}

	// Normalize for case-insensitive comparison
	lowerText := strings.ToLower(text)
	lowerURI := strings.ToLower(expectedURI)

	// Check if the full URI is present
	if strings.Contains(lowerText, lowerURI) {
		return nil
	}

	// Also check without scheme (http:// or https://)
	uriWithoutScheme := strings.TrimPrefix(lowerURI, "https://")
	uriWithoutScheme = strings.TrimPrefix(uriWithoutScheme, "http://")

	if strings.Contains(lowerText, uriWithoutScheme) {
		return nil
	}

	// URI not found
	return DomainError{
		Op:  op,
		Err: fmt.Errorf("%w: URI %q not found in content", ErrURINotFound, expectedURI),
	}
}

// ValidateGeneratedContent performs comprehensive validation on generated content.
// This is a convenience function that combines all validation checks.
func ValidateGeneratedContent(text string, platform string, hashtags []string, postURI string) error {
	const op = "ValidateGeneratedContent"

	// Validate content quality
	if err := ValidateContentQuality(text); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	// Validate hashtags
	if err := ValidateHashtags(hashtags, platform); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	// Validate URI presence (if provided)
	if postURI != "" {
		if err := ContainsURI(text, postURI); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}
