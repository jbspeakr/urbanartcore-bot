package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coverPost(t *testing.T, article Article) *Post {
	t.Helper()
	post, err := NewPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14")
	require.NoError(t, err)
	return post.WithArticle(article)
}

func TestCoverImage(t *testing.T) {
	post := coverPost(t, Article{
		Title:  "Banksy Revealed!",
		Credit: "Brad Downey",
		Cover:  Cover{URL: "https://www.urbanartcore.com/banksy-revealed/wall.jpg", Alt: "Banksy Revealed!", Width: 1200, Height: 720},
	})

	img, ok := post.CoverImage("A stencilled rat holds a paint roller on a white wall")
	require.True(t, ok)
	assert.Equal(t, Image{
		URL:    "https://www.urbanartcore.com/banksy-revealed/wall.jpg",
		Alt:    "A stencilled rat holds a paint roller on a white wall. Photo: Brad Downey.",
		Width:  1200,
		Height: 720,
	}, img)

	img, _ = post.CoverImage("")
	assert.Equal(t, "Banksy Revealed! Photo: Brad Downey.", img.Alt, "without a description the site's alt text stands in")

	untitled := coverPost(t, Article{Title: "Wall in Berlin", Cover: Cover{URL: "https://x/y.jpg"}})
	img, _ = untitled.CoverImage("  ")
	assert.Equal(t, "Wall in Berlin.", img.Alt, "the title stands in for missing alt text")

	_, ok = coverPost(t, Article{Title: "No photo"}).CoverImage("x")
	assert.False(t, ok)
	var nilPost *Post
	_, ok = nilPost.CoverImage("x")
	assert.False(t, ok)
}

func TestAltText(t *testing.T) {
	assert.Equal(t, "A mural. Photo: Ian Cox.", AltText("A mural", "Photo: Ian Cox"))
	assert.Equal(t, "A mural! Photos by the artists.", AltText(" A   mural! ", "Photos by the artists"))
	assert.Equal(t, "A mural.", AltText("A mural", ""))

	long := AltText(strings.Repeat("wall ", 400), "Jan")
	assert.LessOrEqual(t, CountGraphemes(long), MaxAltTextGraphemes)
	assert.True(t, strings.HasSuffix(long, "… Photo: Jan."), "the credit survives the cut")
}

func TestGeneratedContent_WithImages(t *testing.T) {
	content, err := NewGeneratedContent("A wall in Berlin.", PlatformMastodon, nil, "", "https://www.urbanartcore.com/x/")
	require.NoError(t, err)
	images := []Image{{URL: "a"}, {URL: ""}, {URL: "b"}, {URL: "c"}, {URL: "d"}, {URL: "e"}}

	content.WithImages(images)
	got := content.Images()
	require.Len(t, got, MaxPostImages, "empty URLs are dropped and at most four are kept")
	assert.Equal(t, "d", got[3].URL)
	got[0].URL = "changed"
	assert.Equal(t, "a", content.Images()[0].URL, "callers get a copy")
}
