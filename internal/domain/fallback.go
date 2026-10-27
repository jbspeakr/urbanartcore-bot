package domain

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fallbackTitleLimit keeps a long title from pushing the fallback text over
// the Bluesky limit once the date line and hashtags are added.
const fallbackTitleLimit = 150

// FallbackContent builds a post from the article's title, artists, place
// and publication month alone, for when no generated draft passes review.
// It states nothing the site does not, so it can be posted unchecked:
//
//	Banksy Revealed!
//	Banksy and Brad Downey in Berlin, September 2011.
//
// Mastodon gets the link on its own line; Bluesky carries it in the card.
func FallbackContent(post *Post, platform string) (*GeneratedContent, error) {
	if post == nil {
		return nil, DomainError{Err: ErrInvalidURI}
	}
	article := post.Article()

	title := strings.TrimSpace(article.Title)
	if title == "" {
		title = titleFromURI(post.URI())
	}
	if utf8.RuneCountInString(title) > fallbackTitleLimit {
		title = strings.TrimSpace(string([]rune(title)[:fallbackTitleLimit-1])) + "…"
	}

	var who string
	switch artists := article.Tags.Artists; len(artists) {
	case 0:
	case 1:
		who = artists[0]
	case 2:
		who = artists[0] + " and " + artists[1]
	default:
		who = artists[0] + ", " + artists[1] + " and others"
	}
	line := post.Date().Format("January 2006") + "."
	switch place := first(article.Tags.Places); {
	case who != "" && place != "":
		line = who + " in " + place + ", " + line
	case who != "":
		line = who + ", " + line
	case place != "":
		line = place + ", " + line
	}

	text := line
	if title != "" {
		text = title + "\n" + line
	}
	if platform == PlatformMastodon {
		text += "\n\n" + post.URI()
	}

	content, err := NewGeneratedContent(text, platform, post.Hashtags(), "fallback template", post.URI())
	if err != nil {
		return nil, err
	}
	content.fallback = true
	return content, nil
}

// titleFromURI turns the post's slug into a title, for example
// "https://www.urbanartcore.com/banksy-in-new-york/" into "Banksy In New York".
func titleFromURI(uri string) string {
	path := uri
	if parsed, err := url.Parse(uri); err == nil {
		path = parsed.Path
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	words := strings.FieldsFunc(strings.TrimSuffix(segments[len(segments)-1], ".html"), func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, word := range words {
		r, size := utf8.DecodeRuneInString(word)
		words[i] = string(unicode.ToUpper(r)) + word[size:]
	}
	return strings.Join(words, " ")
}
