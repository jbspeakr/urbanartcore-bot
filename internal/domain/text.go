package domain

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

// ContentLanguage is the BCP 47 language tag of every published post.
// Bluesky receives it as langs, Mastodon as language.
const ContentLanguage = "en"

// UserAgent identifies the bot to the sites it reads, naming its repository
// as the site's guidance asks.
const UserAgent = "urbanartcore-bot (+https://github.com/jbspeakr/urbanartcore-bot)"

// Platform text limits as the platforms count them.
const (
	// BlueskyGraphemeLimit is the maximum number of grapheme clusters in a Bluesky post.
	BlueskyGraphemeLimit = 300
	// BlueskyByteLimit is the maximum number of UTF-8 bytes in a Bluesky post.
	BlueskyByteLimit = 3000
	// MastodonURLLength is the number of characters Mastodon counts for any URL.
	MastodonURLLength = 23
)

// urlPattern matches http and https URLs up to the next whitespace.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// CountGraphemes returns the number of user-perceived characters in text,
// which is how Bluesky counts its 300 character limit.
func CountGraphemes(text string) int {
	return uniseg.GraphemeClusterCount(text)
}

// CountMastodonCharacters returns the length Mastodon counts for a status:
// grapheme clusters, with every URL counted as MastodonURLLength.
func CountMastodonCharacters(text string) int {
	urls := urlPattern.FindAllString(text, -1)
	withoutURLs := urlPattern.ReplaceAllString(text, "")
	return CountGraphemes(withoutURLs) + len(urls)*MastodonURLLength
}

// StripURLs removes http and https URLs from text, keeping line breaks
// and collapsing the spaces left behind.
func StripURLs(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		words := strings.Fields(line)
		filtered := make([]string, 0, len(words))

		for _, word := range words {
			if !strings.HasPrefix(word, "http://") && !strings.HasPrefix(word, "https://") {
				filtered = append(filtered, word)
			}
		}

		if len(filtered) > 0 {
			result = append(result, strings.Join(filtered, " "))
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// RenderBlueskyText builds the final Bluesky post text: URLs are removed,
// because the link travels in the embed card, and hashtags are appended so
// that facets can tag them.
func RenderBlueskyText(text string, hashtags []string) string {
	text = StripURLs(text)
	if len(hashtags) == 0 {
		return text
	}

	hashtagStr := strings.Join(hashtags, " ")
	if text == "" {
		return hashtagStr
	}
	return text + " " + hashtagStr
}

// RenderMastodonText builds the final Mastodon status text, with the
// hashtags on their own last line.
func RenderMastodonText(text string, hashtags []string) string {
	if len(hashtags) == 0 {
		return text
	}
	return text + "\n\n" + strings.Join(hashtags, " ")
}

// RenderText builds the final text for a platform from generated text and hashtags.
func RenderText(text, platform string, hashtags []string) string {
	switch strings.ToLower(platform) {
	case PlatformBluesky:
		return RenderBlueskyText(text, hashtags)
	case PlatformMastodon:
		return RenderMastodonText(text, hashtags)
	default:
		return text
	}
}
