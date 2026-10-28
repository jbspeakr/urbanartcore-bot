package domain

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) time.Time {
	d, _ := time.Parse(DateLayout, s)
	return d
}

func TestRhythm(t *testing.T) {
	cases := map[string]string{
		"2026-11-01": ProgramBackThen,   // first of the month, a Sunday
		"2026-11-02": ProgramOnThisDay,  // Monday
		"2026-11-06": ProgramOnThisDay,  // Friday
		"2026-11-07": ProgramPlace,      // Saturday, ISO week 45
		"2026-11-14": ProgramArtist,     // Saturday, ISO week 46
		"2026-11-08": ProgramCollection, // second Sunday
		"2026-11-15": ProgramOnThisDay,  // third Sunday
		"2026-11-22": ProgramCollection, // fourth Sunday
		"2026-11-29": ProgramOnThisDay,  // fifth Sunday
	}
	for date, want := range cases {
		assert.Equal(t, want, Rhythm(day(date)), date)
	}
}

func TestIsFeatureProgram(t *testing.T) {
	assert.True(t, IsFeatureProgram(ProgramCollection))
	assert.True(t, IsFeatureProgram(ProgramBackThen))
	assert.False(t, IsFeatureProgram(ProgramPlace))
	assert.False(t, IsFeatureProgram(ProgramOnThisDay))
}

func TestChoosePlacePost(t *testing.T) {
	var posts []*Post
	for i := 0; i < PlaceMinPosts; i++ {
		article := Article{Title: fmt.Sprintf("Berlin %d", i), Tags: Tags{Places: []string{"Berlin"}}}
		if i == 3 {
			article.Tags.Artists = []string{"Blu"}
		}
		posts = append(posts, planPost(t, fmt.Sprintf("berlin-%d", i), fmt.Sprintf("2010-03-%02d", i+1), article))
	}
	for i := 0; i < PlaceMinPosts+2; i++ {
		posts = append(posts, planPost(t, fmt.Sprintf("london-%d", i), fmt.Sprintf("2011-04-%02d", i+1),
			Article{Tags: Tags{Places: []string{"London"}}}))
	}
	posts = append(posts, planPost(t, "bristol", "2012-01-01", Article{Tags: Tags{Places: []string{"Bristol"}}}))

	place, best, ok := ChoosePlacePost(posts, planDay, nil, nil)
	require.True(t, ok)
	assert.Equal(t, "London", place, "more posts first among places never featured")
	assert.Equal(t, "place London", best.Reasons[0])

	place, best, ok = ChoosePlacePost(posts, planDay, nil, map[string]time.Time{"London": planDay.AddDate(0, -1, 0)})
	require.True(t, ok)
	assert.Equal(t, "Berlin", place, "a place rests for half a year")
	assert.Equal(t, "https://www.urbanartcore.com/berlin-3/", best.Post.URI(), "the place's best post")

	_, _, ok = ChoosePlacePost(posts, planDay, nil, map[string]time.Time{"London": planDay, "Berlin": planDay})
	assert.False(t, ok, "Bristol has too few posts")
}
