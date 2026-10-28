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

func TestCollectionThread(t *testing.T) {
	thread, err := CollectionThread(bristolCollection())
	require.NoError(t, err)
	assert.Equal(t, ProgramCollection, thread.Program)
	assert.Equal(t, "One street, two summers.\nThree reports on See No Evil, read in the order they were published.", thread.Root.Text)
	require.Len(t, thread.Replies, 2)
	assert.Equal(t, "1/2 Europe’s Largest Street Art Event, October 2011\nThe 2011 edition: 72 artists and some 13,000 spray cans on Nelson Street.", thread.Replies[0].Text)
	assert.Equal(t, "2/2 See No Evil Bristol - Final Walls\nThe finished walls, photographed by Ian Cox.", thread.Replies[1].Text)

	bluesky, err := thread.Contents(PlatformBluesky)
	require.NoError(t, err)
	require.Len(t, bluesky, 3)
	assert.Equal(t, "https://www.urbanartcore.com/collections/bristol/", bluesky[0].URI(), "the root's card links the collection")
	assert.Equal(t, []string{"#StreetArt", "#Graffiti"}, bluesky[0].Hashtags())
	assert.Empty(t, bluesky[1].Hashtags(), "replies carry no hashtags")
	assert.NotContains(t, bluesky[1].RenderedText(), "https://", "Bluesky shows the link as a card")

	mastodon, err := thread.Contents(PlatformMastodon)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(mastodon[2].Text(), "\nhttps://www.urbanartcore.com/see-no-evil-bristol-final-walls/"))

	_, err = CollectionThread(Collection{Slug: "empty"})
	assert.ErrorIs(t, err, ErrNothingToFeature)
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

func TestArtistThread(t *testing.T) {
	thread, err := ArtistThread(artistPage(t, "Nychos", 2010, 2010, 2011, 2012, 2012))
	require.NoError(t, err)
	assert.Equal(t, "Nychos in the Urban Art Core archive: 5 posts between 2010 and 2012. The first, one from the middle, and the last.", thread.Root.Text)
	assert.Equal(t, []string{"#Nychos", "#StreetArt", "#Graffiti"}, thread.Hashtags)
	require.Len(t, thread.Replies, 3)
	assert.Equal(t, "May 2010: Nychos post 0", thread.Replies[0].Text)
	assert.Equal(t, "May 2011: Nychos post 2", thread.Replies[1].Text)
	assert.Equal(t, "May 2012: Nychos post 4", thread.Replies[2].Text)

	_, err = ArtistThread(artistPage(t, "Solo", 2010, 2011))
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

func TestBackThenThread(t *testing.T) {
	posts := []*Post{
		planPost(t, "oct-2009", "2009-10-03", Article{Title: "October 2009"}),
		planPost(t, "oct-2010-plain", "2010-10-05", Article{Title: "Plain"}),
		planPost(t, "oct-2010-artist", "2010-10-06", Article{Title: "With artist", Tags: Tags{Artists: []string{"Roa"}}}),
		planPost(t, "nov-2010", "2010-11-01", Article{Title: "November"}),
		planPost(t, "oct-2011", "2011-10-09", Article{Title: "October 2011"}),
	}
	thread, err := BackThenThread(posts, time.October, planDay, nil)
	require.NoError(t, err)
	assert.Equal(t, "October in the Urban Art Core archive, year by year.", thread.Root.Text)
	require.Len(t, thread.Replies, 3)
	assert.Equal(t, "October 2009: October 2009", thread.Replies[0].Text)
	assert.Equal(t, "October 2010: With artist", thread.Replies[1].Text, "the best post of each year")
	assert.Equal(t, "October 2011: October 2011", thread.Replies[2].Text)

	_, err = BackThenThread(posts, time.November, planDay, nil)
	assert.ErrorIs(t, err, ErrNothingToFeature)
}

func TestThreadContents_CutsLongText(t *testing.T) {
	thread := Thread{Root: ThreadPart{Text: strings.Repeat("wall ", 120), Link: "https://www.urbanartcore.com/x/"}}
	contents, err := thread.Contents(PlatformBluesky)
	require.NoError(t, err)
	assert.LessOrEqual(t, contents[0].CharacterCount(), BlueskyCharacterLimit)
	assert.True(t, strings.HasSuffix(contents[0].Text(), "…"))
}
