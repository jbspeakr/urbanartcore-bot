package publisher

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildFacets tests facet generation for hashtags
func TestBuildFacets(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		hashtags      []string
		expectedCount int
	}{
		{
			name:          "empty text",
			text:          "",
			hashtags:      []string{"#test"},
			expectedCount: 0,
		},
		{
			name:          "no hashtags",
			text:          "Some text",
			hashtags:      []string{},
			expectedCount: 0,
		},
		{
			name:          "single hashtag",
			text:          "Check out #StreetArt",
			hashtags:      []string{"#StreetArt"},
			expectedCount: 1,
		},
		{
			name:          "multiple hashtags",
			text:          "#Art is #Amazing and #Creative",
			hashtags:      []string{"#Art", "#Amazing", "#Creative"},
			expectedCount: 3,
		},
		{
			name:          "duplicate hashtag in text",
			text:          "#Art and more #Art",
			hashtags:      []string{"#Art"},
			expectedCount: 2,
		},
		{
			name:          "empty hashtag in list",
			text:          "Some #text",
			hashtags:      []string{"", "#text", "   "},
			expectedCount: 1,
		},
		{
			name:          "case insensitive match",
			text:          "Amazing #streetart",
			hashtags:      []string{"#StreetArt"},
			expectedCount: 1,
		},
		{
			name:          "emoji in text",
			text:          "🎨 Amazing #Art 🖼️",
			hashtags:      []string{"#Art"},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facets := BuildFacets(tt.text, tt.hashtags)
			assert.Len(t, facets, tt.expectedCount)

			// Validate all facets
			if len(facets) > 0 {
				err := ValidateFacets(tt.text, facets)
				assert.NoError(t, err)

				// Check facet structure
				for _, facet := range facets {
					assert.NotNil(t, facet.Feature)
					assert.Equal(t, "app.bsky.richtext.facet#tag", facet.Feature["$type"])
					assert.NotEmpty(t, facet.Feature["tag"])
				}
			}
		})
	}
}

// TestFindHashtagPositions tests hashtag position finding
func TestFindHashtagPositions(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		hashtag       string
		expectedCount int
	}{
		{
			name:          "empty text",
			text:          "",
			hashtag:       "#test",
			expectedCount: 0,
		},
		{
			name:          "empty hashtag",
			text:          "Some text",
			hashtag:       "",
			expectedCount: 0,
		},
		{
			name:          "hashtag not found",
			text:          "Some text",
			hashtag:       "#notfound",
			expectedCount: 0,
		},
		{
			name:          "single occurrence",
			text:          "Check out #StreetArt",
			hashtag:       "#StreetArt",
			expectedCount: 1,
		},
		{
			name:          "multiple occurrences",
			text:          "#Art and more #Art everywhere #Art",
			hashtag:       "#Art",
			expectedCount: 3,
		},
		{
			name:          "case insensitive",
			text:          "Amazing #streetart here",
			hashtag:       "#STREETART",
			expectedCount: 1,
		},
		{
			name:          "with emoji",
			text:          "🎨 #Art 🖼️",
			hashtag:       "#Art",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positions := findHashtagPositions(tt.text, tt.hashtag)
			assert.Len(t, positions, tt.expectedCount)

			// Verify byte ranges are valid
			for _, pos := range positions {
				assert.True(t, isValidByteRange(tt.text, pos.Start, pos.End))
			}
		})
	}
}

// TestCreateHashtagFacet tests hashtag facet creation
func TestCreateHashtagFacet(t *testing.T) {
	tests := []struct {
		name     string
		range_   ByteRange
		hashtag  string
		expected string
	}{
		{
			name:     "hashtag with # prefix",
			range_:   ByteRange{Start: 0, End: 10},
			hashtag:  "#StreetArt",
			expected: "StreetArt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facet := createHashtagFacet(tt.range_, tt.hashtag)

			assert.Equal(t, tt.range_.Start, facet.Index.Start)
			assert.Equal(t, tt.range_.End, facet.Index.End)
			assert.Equal(t, "app.bsky.richtext.facet#tag", facet.Feature["$type"])
			assert.Equal(t, tt.expected, facet.Feature["tag"])
		})
	}
}

// TestCreateLinkFacet tests link facet creation
func TestCreateLinkFacet(t *testing.T) {
	range_ := ByteRange{Start: 10, End: 40}
	uri := "https://example.com/post"

	facet := CreateLinkFacet(range_, uri)

	assert.Equal(t, range_.Start, facet.Index.Start)
	assert.Equal(t, range_.End, facet.Index.End)
	assert.Equal(t, "app.bsky.richtext.facet#link", facet.Feature["$type"])
	assert.Equal(t, uri, facet.Feature["uri"])
}

// TestIsValidByteRange tests byte range validation
func TestIsValidByteRange(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		start    int
		end      int
		expected bool
	}{
		{
			name:     "valid range",
			text:     "Hello world",
			start:    0,
			end:      5,
			expected: true,
		},
		{
			name:     "negative start",
			text:     "Hello",
			start:    -1,
			end:      5,
			expected: false,
		},
		{
			name:     "negative end",
			text:     "Hello",
			start:    0,
			end:      -1,
			expected: false,
		},
		{
			name:     "start >= end",
			text:     "Hello",
			start:    5,
			end:      5,
			expected: false,
		},
		{
			name:     "end exceeds text length",
			text:     "Hello",
			start:    0,
			end:      100,
			expected: false,
		},
		{
			name:     "emoji boundary (valid)",
			text:     "🎨 Art",
			start:    0,
			end:      4, // 🎨 is 4 bytes
			expected: true,
		},
		{
			name:     "splits UTF-8 sequence (invalid)",
			text:     "🎨 Art",
			start:    0,
			end:      2, // Splits emoji (4 bytes)
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidByteRange(tt.text, tt.start, tt.end)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildExternalEmbed tests external embed creation
func TestBuildExternalEmbed(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		title       string
		description string
		thumb       string
		hasThumb    bool
	}{
		{
			name:        "without thumbnail",
			uri:         "https://example.com",
			title:       "Test Title",
			description: "Test Description",
			thumb:       "",
			hasThumb:    false,
		},
		{
			name:        "with thumbnail",
			uri:         "https://example.com",
			title:       "Test Title",
			description: "Test Description",
			thumb:       "base64encodeddata",
			hasThumb:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embed := BuildExternalEmbed(tt.uri, tt.title, tt.description, tt.thumb)

			assert.Equal(t, "app.bsky.embed.external", embed["$type"])
			assert.NotNil(t, embed["external"])

			external, ok := embed["external"].(map[string]interface{})
			require.True(t, ok)

			assert.Equal(t, tt.uri, external["uri"])
			assert.Equal(t, tt.title, external["title"])
			assert.Equal(t, tt.description, external["description"])

			if tt.hasThumb {
				assert.NotNil(t, external["thumb"])
			} else {
				assert.Nil(t, external["thumb"])
			}
		})
	}
}

// TestFindLinkPositions tests URL finding in text
func TestFindLinkPositions(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		expectedCount int
		expectedURLs  []string
	}{
		{
			name:          "empty text",
			text:          "",
			expectedCount: 0,
			expectedURLs:  nil,
		},
		{
			name:          "no links",
			text:          "Just plain text",
			expectedCount: 0,
			expectedURLs:  nil,
		},
		{
			name:          "single https link",
			text:          "Check out https://example.com for more",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com"},
		},
		{
			name:          "single http link",
			text:          "Visit http://example.com today",
			expectedCount: 1,
			expectedURLs:  []string{"http://example.com"},
		},
		{
			name:          "www link",
			text:          "Go to www.example.com now",
			expectedCount: 1,
			expectedURLs:  []string{"www.example.com"},
		},
		{
			name:          "multiple links",
			text:          "See https://example.com and http://test.com",
			expectedCount: 2,
			expectedURLs:  []string{"https://example.com", "http://test.com"},
		},
		{
			name:          "link with path",
			text:          "Read https://example.com/blog/post-123",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com/blog/post-123"},
		},
		{
			name:          "link with query params",
			text:          "Visit https://example.com?foo=bar&baz=qux",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com?foo=bar&baz=qux"},
		},
		{
			name:          "link ending with punctuation",
			text:          "Check (https://example.com) or [http://test.com]",
			expectedCount: 2,
			expectedURLs:  []string{"https://example.com", "http://test.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := FindLinkPositions(tt.text)
			assert.Len(t, results, tt.expectedCount)

			if tt.expectedCount > 0 {
				for i, result := range results {
					assert.Equal(t, tt.expectedURLs[i], result.URL)
					assert.True(t, isValidByteRange(tt.text, result.Range.Start, result.Range.End))
				}
			}
		})
	}
}

// TestCountTextBytes tests byte counting
func TestCountTextBytes(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "ASCII only",
			text:     "Hello",
			expected: 5,
		},
		{
			name:     "single emoji",
			text:     "🎨",
			expected: 4, // Emoji is 4 bytes in UTF-8
		},
		{
			name:     "mixed ASCII and emoji",
			text:     "Hello 🎨",
			expected: 10, // 6 ASCII + 4 emoji
		},
		{
			name:     "multiple emoji",
			text:     "🎨🖼️",
			expected: 11, // Actually more bytes due to UTF-8 encoding
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountTextBytes(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncateToBytes tests text truncation
func TestTruncateToBytes(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxBytes  int
		wantBytes int
	}{
		{
			name:      "no truncation needed",
			text:      "Hello",
			maxBytes:  10,
			wantBytes: 5,
		},
		{
			name:      "exact fit",
			text:      "Hello",
			maxBytes:  5,
			wantBytes: 5,
		},
		{
			name:      "truncate ASCII",
			text:      "Hello World",
			maxBytes:  5,
			wantBytes: 5,
		},
		{
			name:      "truncate at emoji boundary",
			text:      "Hello 🎨 World",
			maxBytes:  6, // Should truncate before emoji
			wantBytes: 6,
		},
		{
			name:      "truncate preserving emoji",
			text:      "🎨 Art",
			maxBytes:  10, // Should preserve emoji + space + Art
			wantBytes: 8,  // 4 (emoji) + 1 (space) + 3 (Art)
		},
		{
			name:      "truncate multiple emoji",
			text:      "🎨🖼️ Art",
			maxBytes:  5, // Should get first emoji + space
			wantBytes: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToBytes(tt.text, tt.maxBytes)
			assert.LessOrEqual(t, len(result), tt.maxBytes)
			assert.Equal(t, tt.wantBytes, len(result))

			// Verify result is valid UTF-8
			assert.True(t, len(result) == 0 || result[len(result)-1]&0x80 == 0 || result[len(result)-1]&0xC0 == 0xC0)
		})
	}
}

// TestValidateFacets tests facet validation
func TestValidateFacets(t *testing.T) {
	text := "Check out #StreetArt and #Graffiti"

	tests := []struct {
		name    string
		text    string
		facets  []Facet
		wantErr bool
	}{
		{
			name: "valid facets",
			text: text,
			facets: []Facet{
				{
					Index: ByteRange{Start: 10, End: 20},
					Feature: map[string]string{
						"$type": "app.bsky.richtext.facet#tag",
						"tag":   "StreetArt",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid byte range (negative start)",
			text: text,
			facets: []Facet{
				{
					Index: ByteRange{Start: -1, End: 10},
					Feature: map[string]string{
						"$type": "app.bsky.richtext.facet#tag",
						"tag":   "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid byte range (exceeds text)",
			text: text,
			facets: []Facet{
				{
					Index: ByteRange{Start: 0, End: 1000},
					Feature: map[string]string{
						"$type": "app.bsky.richtext.facet#tag",
						"tag":   "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name:    "empty facets",
			text:    text,
			facets:  []Facet{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFacets(tt.text, tt.facets)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestFindHashtagPositions_EmojiSafety tests that hashtag finding handles emojis safely
func TestFindHashtagPositions_EmojiSafety(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		hashtag       string
		expectedCount int
	}{
		{
			name:          "emoji before hashtag",
			text:          "🎨 #Art is amazing",
			hashtag:       "#Art",
			expectedCount: 1,
		},
		{
			name:          "multiple emoji around hashtag",
			text:          "🎨 #Art 🖼️ #Design 🎭",
			hashtag:       "#Art",
			expectedCount: 1,
		},
		{
			name:          "emoji between hashtags",
			text:          "#Art 🎨 #Design",
			hashtag:       "#Art",
			expectedCount: 1,
		},
		{
			name:          "complex emoji in text",
			text:          "👨‍🎨 #Artist 👩‍🎨",
			hashtag:       "#Artist",
			expectedCount: 1,
		},
		{
			name:          "multiple hashtags with emoji scattered",
			text:          "🎨 #Art amazing 🖼️ #Design great 🎭 #Theater",
			hashtag:       "#Design",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positions := findHashtagPositions(tt.text, tt.hashtag)
			assert.Len(t, positions, tt.expectedCount)

			// Verify all positions are at valid byte boundaries
			for _, pos := range positions {
				assert.True(t, isValidByteRange(tt.text, pos.Start, pos.End))
				// Extract and verify the substring
				extracted := tt.text[pos.Start:pos.End]
				assert.Equal(t, strings.ToLower(tt.hashtag), strings.ToLower(extracted))
			}
		})
	}
}

// TestBuildFacets_EmojiSafety tests that facet building handles emojis safely
func TestBuildFacets_EmojiSafety(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		hashtags      []string
		expectedCount int
	}{
		{
			name:          "emoji before and after hashtag",
			text:          "🎨 #Art 🖼️",
			hashtags:      []string{"#Art"},
			expectedCount: 1,
		},
		{
			name:          "multiple emoji types with hashtags",
			text:          "👨‍🎨 #Artist 🎭 #Theater 🎪",
			hashtags:      []string{"#Artist", "#Theater"},
			expectedCount: 2,
		},
		{
			name:          "duplicate hashtag with emoji",
			text:          "🎨 #Art 🖼️ #Art 🎭",
			hashtags:      []string{"#Art"},
			expectedCount: 2,
		},
		{
			name:          "emoji only with no matching hashtags",
			text:          "🎨 🖼️ 🎭 amazing stuff",
			hashtags:      []string{"#Art"},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facets := BuildFacets(tt.text, tt.hashtags)
			assert.Len(t, facets, tt.expectedCount)

			// Validate all facets
			for _, facet := range facets {
				assert.True(t, isValidByteRange(tt.text, facet.Index.Start, facet.Index.End))
				assert.Equal(t, "app.bsky.richtext.facet#tag", facet.Feature["$type"])
			}
		})
	}
}

// TestFindLinkPositions_EmojiSafety tests that link finding handles emojis safely
func TestFindLinkPositions_EmojiSafety(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		expectedCount int
		expectedURLs  []string
	}{
		{
			name:          "emoji before link",
			text:          "🎨 Check https://example.com for art",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com"},
		},
		{
			name:          "emoji after link",
			text:          "Visit https://example.com 🎨 for inspiration",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com"},
		},
		{
			name:          "emoji between links",
			text:          "See https://art.com 🎨 and https://design.com",
			expectedCount: 2,
			expectedURLs:  []string{"https://art.com", "https://design.com"},
		},
		{
			name:          "complex emoji with link",
			text:          "👨‍🎨 Artist portfolio: https://example.com/portfolio 👩‍🎨",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com/portfolio"},
		},
		{
			name:          "multiple emoji types with link",
			text:          "🎨 🖼️ 🎭 awesome link https://example.com/amazing",
			expectedCount: 1,
			expectedURLs:  []string{"https://example.com/amazing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := FindLinkPositions(tt.text)
			assert.Len(t, results, tt.expectedCount)

			// Verify all positions are valid and URLs match
			for i, result := range results {
				assert.True(t, isValidByteRange(tt.text, result.Range.Start, result.Range.End))
				assert.Equal(t, tt.expectedURLs[i], result.URL)
			}
		})
	}
}
