package domain

import (
	"strings"

	"github.com/rivo/uniseg"
)

// MaxPostImages is how many images a post carries at most; Bluesky and
// Mastodon both allow four.
const MaxPostImages = 4

// MaxAltTextGraphemes caps an image description. Mastodon accepts 1,500
// characters and Bluesky 2,000; a screen reader user wants far fewer.
const MaxAltTextGraphemes = 1000

// Image is a photograph attached to a post, with the text a screen reader
// reads in its place.
type Image struct {
	URL    string
	Alt    string
	Width  int
	Height int
}

// CoverImage returns the article's cover with alt text built from
// description, or from the cover's own alt text or the title when
// description is empty. The photo credit, when the post names one, ends
// the alt text. It reports false when the post has no cover.
func (p *Post) CoverImage(description string) (Image, bool) {
	if p == nil {
		return Image{}, false
	}
	article := p.Article()
	cover := article.Cover
	if strings.TrimSpace(cover.URL) == "" {
		return Image{}, false
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = strings.TrimSpace(cover.Alt)
	}
	if description == "" {
		description = strings.TrimSpace(article.Title)
	}
	return Image{
		URL:    cover.URL,
		Alt:    AltText(description, article.Credit),
		Width:  cover.Width,
		Height: cover.Height,
	}, true
}

// AltText joins a description and a photo credit into alt text of at most
// MaxAltTextGraphemes, keeping the credit whole.
func AltText(description, credit string) string {
	description = sentence(strings.Join(strings.Fields(description), " "))
	credit = strings.Join(strings.Fields(credit), " ")
	if credit != "" && !strings.HasPrefix(strings.ToLower(credit), "photo") {
		credit = "Photo: " + credit
	}
	credit = sentence(credit)

	room := MaxAltTextGraphemes
	if credit != "" {
		room -= CountGraphemes(credit) + 1
	}
	if CountGraphemes(description) > room {
		description = truncateGraphemes(description, room-1) + "…"
	}
	return strings.TrimSpace(description + " " + credit)
}

// sentence ends s with a full stop unless it already ends in punctuation.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	for _, end := range []string{".", "!", "?", "…"} {
		if strings.HasSuffix(s, end) {
			return s
		}
	}
	return s + "."
}

// truncateGraphemes returns the first n grapheme clusters of s, trimmed.
func truncateGraphemes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	g := uniseg.NewGraphemes(s)
	var b strings.Builder
	for i := 0; i < n && g.Next(); i++ {
		b.WriteString(g.Str())
	}
	return strings.TrimSpace(b.String())
}
