package publisher

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ByteRange represents a byte position range in UTF-8 encoded text.
// ATProto facets use byte positions, not character/rune positions.
type ByteRange struct {
	Start int // Start byte position (0-indexed, inclusive)
	End   int // End byte position (0-indexed, exclusive)
}

// Facet represents an ATProto facet with its byte range and feature type.
// Facets are used to add rich metadata to text (hashtags, links, mentions).
type Facet struct {
	Index   ByteRange         // Byte position range in the text
	Feature map[string]string // Feature type and metadata
}

// BuildFacets generates ATProto facets for hashtags and links in the text.
// It identifies hashtag positions and creates facet metadata for rich text rendering.
// Hashtags must be pre-cleaned and start with # (enforced by domain.GeneratedContent).
// Returns a slice of facets sorted by position, or empty slice if no facets found.
func BuildFacets(text string, hashtags []string) []Facet {
	if text == "" || len(hashtags) == 0 {
		return []Facet{}
	}

	facets := make([]Facet, 0, len(hashtags))

	// Find and create facets for each hashtag
	// Note: hashtags are pre-cleaned by domain.GeneratedContent and already have # prefix
	for _, tag := range hashtags {
		if tag == "" {
			continue
		}

		// Find all positions of this hashtag in the text
		positions := findHashtagPositions(text, tag)
		for _, pos := range positions {
			// Validate byte range before creating facet
			if !isValidByteRange(text, pos.Start, pos.End) {
				continue
			}

			facet := createHashtagFacet(pos, tag)
			facets = append(facets, facet)
		}
	}

	return facets
}

// findHashtagPositions locates all occurrences of a hashtag in the text.
// Returns byte ranges for each occurrence, accounting for UTF-8 encoding.
// Case-insensitive search to match hashtags regardless of case.
func findHashtagPositions(text, hashtag string) []ByteRange {
	if text == "" || hashtag == "" {
		return []ByteRange{}
	}

	positions := []ByteRange{}
	lowerText := strings.ToLower(text)
	lowerHashtag := strings.ToLower(hashtag)
	hashtagBytes := len(lowerHashtag)

	// Search for hashtag starting from each byte position
	// Only search from valid rune boundaries
	byteOffset := 0
	for byteOffset < len(text) {
		// Find next occurrence in the remaining text
		idx := strings.Index(lowerText[byteOffset:], lowerHashtag)
		if idx == -1 {
			break
		}

		// The match position is byteOffset + idx (in bytes)
		startByte := byteOffset + idx
		endByte := startByte + hashtagBytes

		// Verify we're at valid rune boundaries
		if isValidByteRange(text, startByte, endByte) {
			positions = append(positions, ByteRange{
				Start: startByte,
				End:   endByte,
			})
		}

		// Move past this match for next search
		byteOffset = startByte + hashtagBytes
	}

	return positions
}

// createHashtagFacet creates an ATProto hashtag facet with the given byte range.
// Hashtags are rendered as clickable tags that link to tag search results.
func createHashtagFacet(byteRange ByteRange, hashtag string) Facet {
	// Remove # prefix for tag value
	tag := strings.TrimPrefix(hashtag, "#")

	return Facet{
		Index: byteRange,
		Feature: map[string]string{
			"$type": "app.bsky.richtext.facet#tag",
			"tag":   tag,
		},
	}
}

// CreateLinkFacet creates an ATProto link facet for external URIs.
// Links are rendered as clickable hyperlinks in the text.
func CreateLinkFacet(byteRange ByteRange, uri string) Facet {
	return Facet{
		Index: byteRange,
		Feature: map[string]string{
			"$type": "app.bsky.richtext.facet#link",
			"uri":   uri,
		},
	}
}

// isValidByteRange validates that a byte range is valid for the given text.
// Checks that start/end are within bounds and don't split UTF-8 sequences.
func isValidByteRange(text string, start, end int) bool {
	textBytes := len(text)

	// Check bounds
	if start < 0 || end < 0 || start >= end || end > textBytes {
		return false
	}

	// Check that start and end don't split UTF-8 sequences
	// They should align with rune boundaries
	if !utf8.RuneStart(text[start]) {
		return false
	}

	// For end position, check the byte before end (last byte of range)
	if end < textBytes && !utf8.RuneStart(text[end]) {
		return false
	}

	return true
}

// BuildExternalEmbed creates an ATProto external embed for link cards.
// External embeds show rich link previews with title, description, and thumbnail.
// Returns a map representing the embed structure for the ATProto API.
func BuildExternalEmbed(uri, title, description, thumbBase64 string) map[string]any {
	embed := map[string]any{
		"$type": "app.bsky.embed.external",
		"external": map[string]any{
			"uri":         uri,
			"title":       title,
			"description": description,
		},
	}

	// Add thumbnail if provided (base64-encoded JPEG)
	if thumbBase64 != "" {
		external := embed["external"].(map[string]any)
		external["thumb"] = map[string]any{
			"$type": "blob",
			"ref":   map[string]string{"$link": thumbBase64},
		}
	}

	return embed
}

// FindLinkPositions locates URLs in text and returns their byte ranges.
// Used to create link facets for clickable URLs in posts.
// Matches http://, https://, and www. prefixes.
// Safe with UTF-8 multi-byte characters and emojis.
func FindLinkPositions(text string) []struct {
	Range ByteRange
	URL   string
} {
	if text == "" {
		return nil
	}

	results := []struct {
		Range ByteRange
		URL   string
	}{}

	// Simple URL detection - matches http://, https://, and www.
	// More sophisticated URL parsing could be added if needed
	patterns := []string{"https://", "http://", "www."}

	byteOffset := 0
	for byteOffset < len(text) {
		foundIdx := -1

		// Find the next URL pattern
		for _, pattern := range patterns {
			idx := strings.Index(text[byteOffset:], pattern)
			if idx != -1 && (foundIdx == -1 || idx < foundIdx) {
				foundIdx = idx
			}
		}

		if foundIdx == -1 {
			break
		}

		startByte := byteOffset + foundIdx

		// Find the end of the URL (space, newline, or end of text)
		// Only examine byte values for ASCII delimiters (safe - no multi-byte chars needed)
		urlEnd := startByte
		for urlEnd < len(text) {
			// Check for ASCII delimiters - these are always single bytes
			b := text[urlEnd]
			if b == ' ' || b == '\n' || b == '\t' || b == ')' || b == ']' {
				break
			}
			// Skip high bytes (continuation bytes of multi-byte UTF-8 sequences)
			// This safely moves through UTF-8 without splitting characters
			if b&0x80 == 0 {
				// ASCII byte - safe to advance
				urlEnd++
			} else if b&0xC0 == 0xC0 {
				// Start of multi-byte sequence - find the end of this rune
				_, size := utf8.DecodeRuneInString(text[urlEnd:])
				if size > 0 {
					urlEnd += size
				} else {
					// Invalid UTF-8 - advance one byte to recover
					urlEnd++
				}
			} else {
				// Continuation byte - advance normally (shouldn't happen at start of loop)
				urlEnd++
			}
		}

		// Validate byte range before extracting URL string
		if isValidByteRange(text, startByte, urlEnd) {
			url := text[startByte:urlEnd]
			results = append(results, struct {
				Range ByteRange
				URL   string
			}{
				Range: ByteRange{Start: startByte, End: urlEnd},
				URL:   url,
			})
		}

		byteOffset = urlEnd
		if byteOffset >= len(text) {
			break
		}
	}

	return results
}

// CountTextBytes returns the byte length of UTF-8 encoded text.
// ATProto character limits are based on byte count, not rune count.
func CountTextBytes(text string) int {
	return len(text)
}

// TruncateToBytes truncates text to fit within a byte limit while preserving UTF-8 validity.
// Ensures the result ends on a rune boundary and doesn't split multi-byte characters.
func TruncateToBytes(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}

	// Find the last valid rune boundary at or before maxBytes
	truncated := text[:maxBytes]
	for len(truncated) > 0 && !utf8.RuneStart(truncated[len(truncated)-1]) {
		truncated = truncated[:len(truncated)-1]
	}

	// Ensure we didn't truncate to empty
	if len(truncated) == 0 && len(text) > 0 {
		// Take at least the first rune
		_, size := utf8.DecodeRuneInString(text)
		if size > 0 && size <= maxBytes {
			return text[:size]
		}
	}

	return truncated
}

// ValidateFacets checks that all facets have valid byte ranges for the text.
// Returns an error if any facet is invalid, nil otherwise.
func ValidateFacets(text string, facets []Facet) error {
	for i, facet := range facets {
		if !isValidByteRange(text, facet.Index.Start, facet.Index.End) {
			return fmt.Errorf("invalid byte range for facet %d: start=%d, end=%d", i, facet.Index.Start, facet.Index.End)
		}
	}
	return nil
}
