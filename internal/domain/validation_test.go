package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHashtags(t *testing.T) {
	tests := []struct {
		name        string
		hashtags    []string
		platform    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid bluesky hashtags - minimum",
			hashtags: []string{"#StreetArt"},
			platform: PlatformBluesky,
			wantErr:  false,
		},
		{
			name:     "valid bluesky hashtags - typical",
			hashtags: []string{"#StreetArt", "#Graffiti"},
			platform: PlatformBluesky,
			wantErr:  false,
		},
		{
			name:     "valid bluesky hashtags - maximum",
			hashtags: []string{"#StreetArt", "#Graffiti", "#UrbanArt"},
			platform: PlatformBluesky,
			wantErr:  false,
		},
		{
			name:     "valid mastodon hashtags - minimum",
			hashtags: []string{"#StreetArt", "#Graffiti"},
			platform: PlatformMastodon,
			wantErr:  false,
		},
		{
			name:     "valid mastodon hashtags - maximum",
			hashtags: []string{"#StreetArt", "#Graffiti", "#UrbanArt", "#Culture"},
			platform: PlatformMastodon,
			wantErr:  false,
		},
		{
			name:     "valid hashtags with underscores",
			hashtags: []string{"#Street_Art", "#Urban_Culture"},
			platform: PlatformBluesky,
			wantErr:  false,
		},
		{
			name:     "valid hashtags without # prefix",
			hashtags: []string{"StreetArt", "Graffiti"},
			platform: PlatformBluesky,
			wantErr:  false,
		},
		{
			name:        "bluesky - too few hashtags",
			hashtags:    []string{},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "at least 1 hashtags",
		},
		{
			name:        "bluesky - too many hashtags",
			hashtags:    []string{"#One", "#Two", "#Three", "#Four"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "maximum 3 hashtags",
		},
		{
			name:        "mastodon - too few hashtags",
			hashtags:    []string{"#OnlyOne"},
			platform:    PlatformMastodon,
			wantErr:     true,
			errContains: "at least 2 hashtags",
		},
		{
			name:        "mastodon - too many hashtags",
			hashtags:    []string{"#One", "#Two", "#Three", "#Four", "#Five"},
			platform:    PlatformMastodon,
			wantErr:     true,
			errContains: "maximum 4 hashtags",
		},
		{
			name:        "invalid hashtag - too short",
			hashtags:    []string{"#A", "#StreetArt"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "too short",
		},
		{
			name:        "invalid hashtag - too long",
			hashtags:    []string{"#" + strings.Repeat("A", 51), "#StreetArt"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "invalid hashtag - contains spaces",
			hashtags:    []string{"#Street Art", "#Graffiti"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "invalid hashtag - contains special characters",
			hashtags:    []string{"#Street-Art", "#Graffiti"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "invalid hashtag - contains emoji",
			hashtags:    []string{"#StreetArt🎨", "#Graffiti"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "duplicate hashtags - exact match",
			hashtags:    []string{"#StreetArt", "#StreetArt"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "appears multiple times",
		},
		{
			name:        "duplicate hashtags - case insensitive",
			hashtags:    []string{"#StreetArt", "#streetart"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "appears multiple times",
		},
		{
			name:        "duplicate hashtags - with and without prefix",
			hashtags:    []string{"#StreetArt", "StreetArt"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "appears multiple times",
		},
		{
			name:        "empty hashtag",
			hashtags:    []string{"#StreetArt", "#"},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "is empty",
		},
		{
			name:        "whitespace-only hashtag",
			hashtags:    []string{"#StreetArt", "   "},
			platform:    PlatformBluesky,
			wantErr:     true,
			errContains: "is empty",
		},
		{
			name:        "invalid platform",
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			platform:    "twitter",
			wantErr:     true,
			errContains: "invalid platform",
		},
		{
			name:        "empty platform",
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			platform:    "",
			wantErr:     true,
			errContains: "invalid platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHashtags(tt.hashtags, tt.platform)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateContentQuality(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid content - simple",
			text:    "This is a valid post about street art.",
			wantErr: false,
		},
		{
			name:    "valid content - with URL",
			text:    "Check out this amazing graffiti piece! https://example.com/post",
			wantErr: false,
		},
		{
			name:    "valid content - with hashtags",
			text:    "Amazing street art in Paris! #StreetArt #Graffiti",
			wantErr: false,
		},
		{
			name:    "valid content - with newlines",
			text:    "First line\nSecond line\nThird line",
			wantErr: false,
		},
		{
			name:    "valid content - with unicode",
			text:    "Street art 🎨 in Paris is amazing! Découvrez l'art urbain.",
			wantErr: false,
		},
		{
			name:    "valid content - minimum length",
			text:    "1234567890", // Exactly 10 characters
			wantErr: false,
		},
		{
			name:        "empty content",
			text:        "",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "whitespace-only content",
			text:        "   \n\t   ",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "too short content",
			text:        "Too short",
			wantErr:     true,
			errContains: "too short",
		},
		{
			name:        "injection - script tag",
			text:        "This is a post with <script>alert('xss')</script> in it.",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "injection - javascript protocol",
			text:        "Click here: javascript:alert('xss')",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "injection - event handler",
			text:        "This has onclick=malicious code in it.",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "injection - iframe",
			text:        "Check this out: <iframe src='evil.com'></iframe>",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "injection - eval call",
			text:        "This has eval(malicious) in the code.",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "excessive control characters",
			text:        "Normal text\x00\x01\x02\x03\x04\x05\x06\x07\x08",
			wantErr:     true,
			errContains: "control characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContentQuality(tt.text)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestContainsURI(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		expectedURI string
		wantErr     bool
		errContains string
	}{
		{
			name:        "URI present - exact match",
			text:        "Check out this post: https://example.com/post",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI present - embedded in text",
			text:        "Amazing street art! Visit https://example.com/post for more details.",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI present - case insensitive",
			text:        "Visit HTTPS://EXAMPLE.COM/POST for details.",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI present - without scheme in text",
			text:        "Check out example.com/post for more info.",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI present - http vs https",
			text:        "Visit http://example.com/post for details.",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI present - with query params",
			text:        "Check this: https://example.com/post?id=123",
			expectedURI: "https://example.com/post",
			wantErr:     false,
		},
		{
			name:        "URI not found - different domain",
			text:        "Check out https://different.com/post",
			expectedURI: "https://example.com/post",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "URI not found - different path",
			text:        "Visit https://example.com/other-post",
			expectedURI: "https://example.com/post",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "URI not found - no URI in text",
			text:        "This is a post about street art with no links.",
			expectedURI: "https://example.com/post",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty text",
			text:        "",
			expectedURI: "https://example.com/post",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "empty expected URI",
			text:        "This is some text.",
			expectedURI: "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "whitespace-only text",
			text:        "   \n\t   ",
			expectedURI: "https://example.com/post",
			wantErr:     true,
			errContains: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ContainsURI(tt.text, tt.expectedURI)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateGeneratedContent(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		platform    string
		hashtags    []string
		postURI     string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid bluesky content",
			text:     "Amazing street art! Check out https://example.com/post #StreetArt #Graffiti",
			platform: PlatformBluesky,
			hashtags: []string{"#StreetArt", "#Graffiti"},
			postURI:  "https://example.com/post",
			wantErr:  false,
		},
		{
			name:     "valid mastodon content",
			text:     "Incredible graffiti piece from 2010. More details at https://example.com/post #StreetArt #Graffiti #UrbanArt",
			platform: PlatformMastodon,
			hashtags: []string{"#StreetArt", "#Graffiti", "#UrbanArt"},
			postURI:  "https://example.com/post",
			wantErr:  false,
		},
		{
			name:     "valid content without URI check",
			text:     "Amazing street art exhibition today! #StreetArt #Graffiti",
			platform: PlatformBluesky,
			hashtags: []string{"#StreetArt", "#Graffiti"},
			postURI:  "", // Empty URI means skip URI check
			wantErr:  false,
		},
		{
			name:        "invalid content - too short",
			text:        "Short",
			platform:    PlatformBluesky,
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			postURI:     "",
			wantErr:     true,
			errContains: "too short",
		},
		{
			name:        "invalid content - injection detected",
			text:        "Check this out: <script>alert('xss')</script> https://example.com/post",
			platform:    PlatformBluesky,
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "injection",
		},
		{
			name:        "invalid hashtags - too many for bluesky",
			text:        "Great street art! https://example.com/post",
			platform:    PlatformBluesky,
			hashtags:    []string{"#One", "#Two", "#Three", "#Four"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "maximum 3 hashtags",
		},
		{
			name:        "invalid hashtags - too few for mastodon",
			text:        "Great street art! https://example.com/post",
			platform:    PlatformMastodon,
			hashtags:    []string{"#OnlyOne"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "at least 2 hashtags",
		},
		{
			name:        "invalid hashtags - bad format",
			text:        "Great street art! https://example.com/post",
			platform:    PlatformBluesky,
			hashtags:    []string{"#Street-Art", "#Graffiti"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "URI not found in content",
			text:        "Great street art exhibition today! #StreetArt #Graffiti",
			platform:    PlatformBluesky,
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "invalid platform",
			text:        "Great street art! https://example.com/post",
			platform:    "twitter",
			hashtags:    []string{"#StreetArt", "#Graffiti"},
			postURI:     "https://example.com/post",
			wantErr:     true,
			errContains: "invalid platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGeneratedContent(tt.text, tt.platform, tt.hashtags, tt.postURI)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkValidateHashtags(b *testing.B) {
	hashtags := []string{"#StreetArt", "#Graffiti", "#UrbanArt"}
	platform := PlatformBluesky

	for b.Loop() {
		_ = ValidateHashtags(hashtags, platform)
	}
}

func BenchmarkValidateContentQuality(b *testing.B) {
	text := "This is a valid post about street art with enough content to pass validation checks."

	for b.Loop() {
		_ = ValidateContentQuality(text)
	}
}

func BenchmarkContainsURI(b *testing.B) {
	text := "Check out this amazing street art post at https://example.com/post for more details!"
	expectedURI := "https://example.com/post"

	for b.Loop() {
		_ = ContainsURI(text, expectedURI)
	}
}

func BenchmarkValidateGeneratedContent(b *testing.B) {
	text := "Amazing street art! Check out https://example.com/post for more details."
	platform := PlatformBluesky
	hashtags := []string{"#StreetArt", "#Graffiti"}
	postURI := "https://example.com/post"

	for b.Loop() {
		_ = ValidateGeneratedContent(text, platform, hashtags, postURI)
	}
}
