package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bristolCollection() Collection {
	return Collection{
		Slug:        "bristol",
		Title:       "One street, two summers",
		Description: "Three reports on See No Evil, read in the order they were published.",
		URL:         "https://www.urbanartcore.com/collections/bristol/",
		Items: []CollectionItem{
			{Position: 1, Title: "Europe’s Largest Street Art Event", URL: "https://www.urbanartcore.com/europe-largest-street-art-event/",
				Note: "The 2011 edition: 72 artists and some 13,000 spray cans on Nelson Street.", Date: time.Date(2011, 10, 17, 0, 0, 0, 0, time.UTC)},
			{Position: 2, Title: "See No Evil Bristol - Final Walls", URL: "https://www.urbanartcore.com/see-no-evil-bristol-final-walls/",
				Note: "The finished walls, photographed by Ian Cox."},
		},
	}
}

func TestCollectionFeature(t *testing.T) {
	c := bristolCollection()
	first := planPost(t, "europe-largest-street-art-event", "2011-10-17", Article{Title: "Europe’s Largest Street Art Event"})
	feature, err := CollectionFeature(c, map[string]*Post{first.URI(): first})
	require.NoError(t, err)
	assert.Equal(t, ProgramCollection, feature.Program)
	assert.Equal(t, c.URL, feature.URL)
	assert.Equal(t, []*Post{first}, feature.Picks, "only items found in the catalogue bring a photo")
	assert.Equal(t, []string{
		"1. Europe’s Largest Street Art Event (October 2011): The 2011 edition: 72 artists and some 13,000 spray cans on Nelson Street.",
		"2. See No Evil Bristol - Final Walls: The finished walls, photographed by Ian Cox.",
	}, feature.Lines)

	post, err := feature.Post()
	require.NoError(t, err)
	assert.Equal(t, c.URL, post.URI(), "the post links the collection")
	assert.Equal(t, "2011-10-17", post.DateString())
	article := post.Article()
	assert.Equal(t, "One street, two summers", article.Title)
	assert.Equal(t, "Three reports on See No Evil, read in the order they were published.", article.Description)
	assert.Contains(t, article.Body, "2. See No Evil Bristol - Final Walls")
	assert.Equal(t, "in October 2011", feature.Period())

	_, err = CollectionFeature(Collection{Slug: "empty"}, nil)
	assert.ErrorIs(t, err, ErrNothingToFeature)

	_, err = Feature{Title: "No photos"}.Post()
	assert.ErrorIs(t, err, ErrNothingToFeature, "a feature needs at least one post")
}

func artistPage(t *testing.T, name string, years ...int) TagPage {
	t.Helper()
	page := TagPage{Name: name, URL: "https://www.urbanartcore.com/" + strings.ToLower(name) + "/"}
	for i, year := range years {
		page.Posts = append(page.Posts, planPost(t, fmt.Sprintf("%s-%d", strings.ToLower(name), i), fmt.Sprintf("%d-05-0%d", year, i%9+1),
			Article{Title: fmt.Sprintf("%s post %d", name, i)}))
	}
	return page
}

func TestArtistFeature(t *testing.T) {
	page := artistPage(t, "Nychos", 2010, 2010, 2011, 2011, 2012, 2012, 2012)
	feature, err := ArtistFeature(page)
	require.NoError(t, err)
	assert.Equal(t, page.URL, feature.URL)
	assert.Equal(t, []string{"Nychos"}, feature.Artists)
	assert.Equal(t, "7 posts about Nychos, published between 2010 and 2012. Below are 4 of them, from the first to the last.", feature.Intro)
	require.Len(t, feature.Picks, 4)
	assert.Equal(t, page.Posts[0], feature.Picks[0], "the first")
	assert.Equal(t, page.Posts[6], feature.Picks[3], "the last")
	assert.Equal(t, "from May 2010 to May 2012", feature.Period())

	post, err := feature.Post()
	require.NoError(t, err)
	assert.Equal(t, "May 2010: Nychos post 0\nMay 2011: Nychos post 2\nMay 2012: Nychos post 4\nMay 2012: Nychos post 6", post.Article().Body)
	assert.Equal(t, []string{"#Nychos", "#StreetArt", "#Graffiti"}, post.Hashtags())

	few, err := ArtistFeature(artistPage(t, "Roa", 2010, 2011, 2012))
	require.NoError(t, err)
	assert.Len(t, few.Picks, 3, "fewer posts than photos: all of them")

	_, err = ArtistFeature(artistPage(t, "Solo", 2010, 2011))
	assert.ErrorIs(t, err, ErrNothingToFeature)
}

func TestChooseArtist(t *testing.T) {
	big := artistPage(t, "Banksy", 2010, 2010, 2011, 2011, 2012, 2012)
	mid := artistPage(t, "Blu", 2010, 2011, 2011, 2012, 2012)
	small := artistPage(t, "Solo", 2010, 2011, 2012)

	got, ok := ChooseArtist([]TagPage{small, mid, big}, planDay, nil)
	require.True(t, ok)
	assert.Equal(t, "Banksy", got.Name, "the artist with the most posts first")

	recent := map[string]time.Time{big.URL: planDay.AddDate(0, -2, 0)}
	got, _ = ChooseArtist([]TagPage{small, mid, big}, planDay, recent)
	assert.Equal(t, "Blu", got.Name, "an artist rests for half a year after a spotlight")

	_, ok = ChooseArtist([]TagPage{small}, planDay, nil)
	assert.False(t, ok, "fewer than five posts is not enough")
}

func TestChooseCollection(t *testing.T) {
	a := bristolCollection()
	b := bristolCollection()
	b.URL = "https://www.urbanartcore.com/collections/stencil-art/"

	got, ok := ChooseCollection([]Collection{a, b}, planDay, map[string]time.Time{a.URL: planDay.AddDate(0, -1, 0)})
	require.True(t, ok)
	assert.Equal(t, b.URL, got.URL, "a collection never featured goes first")

	got, _ = ChooseCollection([]Collection{a, b}, planDay, map[string]time.Time{a.URL: planDay.AddDate(0, -1, 0), b.URL: planDay.AddDate(0, 0, -7)})
	assert.Equal(t, a.URL, got.URL, "then the one featured longest ago")

	_, ok = ChooseCollection([]Collection{{Slug: "empty"}}, planDay, nil)
	assert.False(t, ok)
}

func TestBackThenFeature(t *testing.T) {
	posts := []*Post{
		planPost(t, "oct-2009", "2009-10-03", Article{Title: "October 2009", Description: "A wall in Lisbon."}),
		planPost(t, "oct-2010-plain", "2010-10-05", Article{Title: "Plain"}),
		planPost(t, "oct-2010-artist", "2010-10-06", Article{Title: "With artist", Tags: Tags{Artists: []string{"Roa"}}}),
		planPost(t, "nov-2010", "2010-11-01", Article{Title: "November"}),
		planPost(t, "oct-2011", "2011-10-09", Article{Title: "October 2011"}),
	}
	feature, err := BackThenFeature(posts, time.October, planDay, nil)
	require.NoError(t, err)
	assert.Equal(t, ArchiveURL, feature.URL)
	require.Len(t, feature.Picks, 3)
	assert.Equal(t, "With artist", feature.Picks[1].Article().Title, "the best post of each year")

	post, err := feature.Post()
	require.NoError(t, err)
	assert.Equal(t, "October 2009: October 2009. A wall in Lisbon.\nOctober 2010: With artist\nOctober 2011: October 2011", post.Article().Body)

	_, err = BackThenFeature(posts, time.November, planDay, nil)
	assert.ErrorIs(t, err, ErrNothingToFeature)
}

func TestRetarget(t *testing.T) {
	approved, err := NewGeneratedContent("KR styled a Mini Cooper in Hamburg.", PlatformBluesky, []string{"#KR", "#StreetArt"}, "p", "https://www.urbanartcore.com/krink-mini/")
	require.NoError(t, err)
	approved = approved.WithClaims([]string{"KR styled a Mini Cooper."})

	mastodon, err := Retarget(approved, PlatformMastodon)
	require.NoError(t, err)
	assert.Equal(t, "KR styled a Mini Cooper in Hamburg.\n\nhttps://www.urbanartcore.com/krink-mini/", mastodon.Text())
	assert.Equal(t, PlatformMastodon, mastodon.Platform())
	assert.Equal(t, approved.Hashtags(), mastodon.Hashtags())
	assert.Equal(t, approved.Claims(), mastodon.Claims())
	assert.False(t, mastodon.IsFallback())

	bluesky, err := Retarget(mastodon, PlatformBluesky)
	require.NoError(t, err)
	assert.Equal(t, "KR styled a Mini Cooper in Hamburg.", bluesky.Text(), "Bluesky leaves the link to the card")

	_, err = Retarget(nil, PlatformMastodon)
	assert.Error(t, err)
}
