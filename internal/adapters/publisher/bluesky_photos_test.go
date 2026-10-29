package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

func TestPhotoText(t *testing.T) {
	uri := "https://www.urbanartcore.com/collections/stencil-art/"
	text, facets, ok := photoText("Stencils from Paris to Berlin.", uri, []string{"#Stencil", "#StreetArt"})
	require.True(t, ok)
	assert.Equal(t, "Stencils from Paris to Berlin.\nurbanartcore.com/collections/stencil-art\n\n#Stencil #StreetArt", text)
	require.Len(t, facets, 3)
	link := facets[2]
	assert.Equal(t, "app.bsky.richtext.facet#link", link.Feature["$type"])
	assert.Equal(t, uri, link.Feature["uri"])
	assert.Equal(t, "urbanartcore.com/collections/stencil-art", text[link.Index.Start:link.Index.End])
	require.NoError(t, ValidateFacets(text, facets))

	long := strings.Repeat("wall ", 52) // 259 graphemes: the link fits, the hashtags do not
	text, facets, ok = photoText(long, uri, []string{"#Stencil", "#StreetArt"})
	require.True(t, ok)
	assert.NotContains(t, text, "#", "hashtags are dropped before the link")
	assert.LessOrEqual(t, domain.CountGraphemes(text), domain.BlueskyCharacterLimit)
	assert.Len(t, facets, 1)

	_, _, ok = photoText(strings.Repeat("wall ", 60), uri, nil)
	assert.False(t, ok, "no room for the link")
}

func TestDisplayLink(t *testing.T) {
	assert.Equal(t, "urbanartcore.com/archives", displayLink("https://www.urbanartcore.com/archives/"))
	assert.Equal(t, "bsky.app", displayLink("https://bsky.app"))
}

func TestBlueskyAdapter_PublishPhotoPost(t *testing.T) {
	var img bytes.Buffer
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	pixels.Set(1, 1, color.White)
	require.NoError(t, jpeg.Encode(&img, pixels, nil))

	var record map[string]any
	uploads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_ = json.NewEncoder(w).Encode(map[string]any{"accessJwt": "a", "refreshJwt": "r", "handle": "h", "did": "did:plc:x"})
		case "/xrpc/com.atproto.repo.uploadBlob":
			uploads++
			_ = json.NewEncoder(w).Encode(map[string]any{"blob": map[string]any{"ref": map[string]any{"$link": "bafyimg"}}})
		case "/xrpc/com.atproto.repo.createRecord":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			record, _ = body["record"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{"uri": "at://did:plc:x/app.bsky.feed.post/abc", "cid": "bafy"})
		case "/a.jpg", "/b.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(img.Bytes())
		case "/missing.jpg":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	uri := "https://www.urbanartcore.com/collections/stencil-art/"
	content, err := domain.NewGeneratedContent("Stencils from Paris to Berlin.", domain.PlatformBluesky, []string{"#StreetArt"}, "p", uri)
	require.NoError(t, err)
	content = content.WithImages([]domain.Image{
		{URL: server.URL + "/a.jpg", Alt: "A stencil portrait on a door.", Width: 1200, Height: 800},
		{URL: server.URL + "/missing.jpg", Alt: "Gone."},
		{URL: server.URL + "/b.jpg", Alt: "Stencils on a Berlin wall."},
	})

	adapter := &BlueskyAdapter{handle: "h", password: "p", pdsHost: server.URL, httpClient: &http.Client{}}
	result, err := adapter.Publish(context.Background(), content)
	require.NoError(t, err)
	require.True(t, result.IsSuccess())

	assert.Equal(t, "Stencils from Paris to Berlin.\nurbanartcore.com/collections/stencil-art\n\n#StreetArt", record["text"])
	embed := record["embed"].(map[string]any)
	assert.Equal(t, "app.bsky.embed.images", embed["$type"])
	images := embed["images"].([]any)
	require.Len(t, images, 2, "the photo that failed to download is left out")
	assert.Equal(t, 2, uploads)
	first := images[0].(map[string]any)
	assert.Equal(t, "A stencil portrait on a door.", first["alt"])
	assert.Equal(t, map[string]any{"width": float64(1200), "height": float64(800)}, first["aspectRatio"])
	assert.Len(t, record["facets"], 2, "the hashtag and the link")
}
