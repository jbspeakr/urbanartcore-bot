package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func planPost(t *testing.T, slug, date string, article Article) *Post {
	t.Helper()
	post, err := NewPost("https://www.urbanartcore.com/"+slug+"/", date)
	require.NoError(t, err)
	return post.WithArticle(article)
}

var planDay = time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

func TestScoreOnThisDay(t *testing.T) {
	rich := planPost(t, "banksy-revealed", "2011-09-14", Article{
		Description: "An original, eight years old Banksy mural was recently revealed by Berlin-based artist Brad Downey.",
		Tags:        Tags{Artists: []string{"Banksy"}, Places: []string{"Berlin"}},
	})
	c := ScoreOnThisDay(rich, planDay, nil)
	assert.Equal(t, 6.0, c.Score)
	assert.Equal(t, []string{"artist", "place", "summary", "15 years"}, c.Reasons)

	bare := planPost(t, "thalys", "2009-09-14", Article{})
	assert.Equal(t, RestFloor, ScoreOnThisDay(bare, planDay, nil).Score)

	recent := map[string]time.Time{rich.URI(): planDay.AddDate(0, -3, 0)}
	c = ScoreOnThisDay(rich, planDay, recent)
	assert.Zero(t, c.Score)
	assert.Equal(t, []string{"featured 2026-06-14"}, c.Reasons)

	yearAgo := map[string]time.Time{rich.URI(): planDay.Add(-RepeatWindow)}
	assert.Equal(t, 6.0, ScoreOnThisDay(rich, planDay, yearAgo).Score, "after a year a post may come back")

	assert.Zero(t, ScoreOnThisDay(nil, planDay, nil).Score)
}

func TestBest(t *testing.T) {
	a := planPost(t, "a", "2010-09-14", Article{})
	b := planPost(t, "b", "2010-09-14", Article{})
	strong := planPost(t, "strong", "2010-09-14", Article{Tags: Tags{Artists: []string{"Blu"}}})

	best, ok := Best([]Candidate{ScoreOnThisDay(a, planDay, nil), ScoreOnThisDay(strong, planDay, nil)}, planDay, nil)
	require.True(t, ok)
	assert.Equal(t, strong.URI(), best.Post.URI(), "the higher score wins")

	last := map[string]time.Time{a.URI(): planDay.AddDate(-2, 0, 0)}
	best, _ = Best([]Candidate{{Post: a, Score: 1}, {Post: b, Score: 1}}, planDay, last)
	assert.Equal(t, b.URI(), best.Post.URI(), "on a tie, a post never featured goes first")

	last[b.URI()] = planDay.AddDate(-3, 0, 0)
	best, _ = Best([]Candidate{{Post: a, Score: 1}, {Post: b, Score: 1}}, planDay, last)
	assert.Equal(t, b.URI(), best.Post.URI(), "then the one featured longest ago")

	first, _ := Best([]Candidate{{Post: a, Score: 1}, {Post: b, Score: 1}}, planDay, nil)
	again, _ := Best([]Candidate{{Post: b, Score: 1}, {Post: a, Score: 1}}, planDay, nil)
	assert.Equal(t, first.Post.URI(), again.Post.URI(), "the remaining tie-break does not depend on order")

	_, ok = Best([]Candidate{{Post: a, Score: 0}, {Post: nil, Score: 5}}, planDay, nil)
	assert.False(t, ok, "nothing at or above the floor is a rest day")
}

func TestLastFeatured(t *testing.T) {
	last := LastFeatured([]PublishRecord{
		{Date: "2026-01-02", PostURI: "x"},
		{Date: "2026-03-04", PostURI: "x"},
		{Date: "2026-02-01", PostURI: "x"},
		{Date: "bad", PostURI: "y"},
	})
	assert.Equal(t, map[string]time.Time{"x": time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)}, last)
}
