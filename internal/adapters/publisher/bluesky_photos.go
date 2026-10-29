package publisher

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

const (
	// minPhotoPostImages is how many photos turn a post into a photo post.
	// With one, the link card shows the cover as its thumbnail anyway.
	minPhotoPostImages = 2
	// maxBlueskyImageBytes is the largest image blob a Bluesky post accepts.
	maxBlueskyImageBytes = 1_000_000
)

// photoPost builds a Bluesky photo post: an images embed with the content's
// photos and their alt text, and the link as its own line of the text, since
// a post holds one embed and the photos take it. ok is false when the content
// has fewer than minPhotoPostImages photos, the text with the link does not
// fit, or no photo could be uploaded; the caller then posts the link card.
func (a *BlueskyAdapter) photoPost(ctx context.Context, content *domain.GeneratedContent) (text string, facets []Facet, embed map[string]any, ok bool) {
	images := content.Images()
	if len(images) < minPhotoPostImages {
		return "", nil, nil, false
	}
	text, facets, ok = photoText(domain.StripURLs(content.Text()), content.URI(), content.Hashtags())
	if !ok {
		slog.Default().Warn("Photo post text does not fit with its link, posting the link card",
			slog.String("uri", content.URI()))
		return "", nil, nil, false
	}

	var entries []map[string]any
	for _, image := range images {
		blob, err := a.downloadAndEncodeThumbnail(ctx, image.URL)
		if err != nil || blob == nil {
			attrs := []any{slog.String("image", image.URL)}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
			}
			slog.Default().Warn("Could not upload a photo, leaving it out", attrs...)
			continue
		}
		if size, _ := blob["size"].(int); size > maxBlueskyImageBytes {
			slog.Default().Warn("Photo too large for Bluesky, leaving it out",
				slog.String("image", image.URL), slog.Int("size", size))
			continue
		}
		entry := map[string]any{"image": blob, "alt": image.Alt}
		if image.Width > 0 && image.Height > 0 {
			entry["aspectRatio"] = map[string]int{"width": image.Width, "height": image.Height}
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		slog.Default().Warn("No photo could be uploaded, posting the link card", slog.String("uri", content.URI()))
		return "", nil, nil, false
	}
	return text, facets, map[string]any{"$type": "app.bsky.embed.images", "images": entries}, true
}

// photoText lays out a photo post's text: the body, the link shortened to
// its host and path on its own line, then the hashtags. Hashtags are dropped
// from the end until the text fits; ok is false when even the body and the
// link alone are too long.
func photoText(body, uri string, hashtags []string) (string, []Facet, bool) {
	body = strings.TrimSpace(body)
	link := displayLink(uri)
	for kept := len(hashtags); kept >= 0; kept-- {
		text := body + "\n" + link
		if kept > 0 {
			text += "\n\n" + strings.Join(hashtags[:kept], " ")
		}
		if domain.CountGraphemes(text) > domain.BlueskyCharacterLimit {
			continue
		}
		start := len(body) + 1
		facets := append(BuildFacets(text, hashtags[:kept]), CreateLinkFacet(ByteRange{Start: start, End: start + len(link)}, uri))
		return text, facets, true
	}
	return "", nil, false
}

// displayLink is how a link reads in a post: host and path without the
// scheme, "www." or a trailing slash, as Bluesky's own apps shorten links.
func displayLink(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Host == "" {
		return uri
	}
	return strings.TrimSuffix(strings.TrimPrefix(parsed.Host, "www.")+parsed.Path, "/")
}
