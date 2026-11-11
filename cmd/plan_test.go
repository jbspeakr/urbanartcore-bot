package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

func TestParsePlanStart(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	got, err := parsePlanStart("", now)
	require.NoError(t, err)
	assert.Equal(t, now, got)

	got, err = parsePlanStart("12-01", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-12-01", got.Format(domain.DateLayout))

	got, err = parsePlanStart("2027-01-05", now)
	require.NoError(t, err)
	assert.Equal(t, "2027-01-05", got.Format(domain.DateLayout))

	_, err = parsePlanStart("1.12.", now)
	assert.Error(t, err)
}

func TestWritePlan(t *testing.T) {
	post, err := domain.NewPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14")
	require.NoError(t, err)
	post = post.WithArticle(domain.Article{Title: "Banksy Revealed!"})
	day := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	var out bytes.Buffer
	require.NoError(t, writePlan(&out, []domain.Slot{
		{Date: day, Program: domain.ProgramOnThisDay, Post: post, Score: 4, Reasons: []string{"artist", "15 years"}},
		{Date: day.AddDate(0, 0, 1), Program: domain.ProgramRestDay, Reasons: []string{"no anniversary post"}},
	}))
	text := out.String()
	assert.Contains(t, text, "2026-09-14  Mon  on-this-day  4      Banksy Revealed! (2011)  artist, 15 years")
	assert.Contains(t, text, "2026-09-15  Tue  rest-day     -      -                        no anniversary post")
	assert.Contains(t, text, "2 days: 1 single posts, 0 features, 1 rest days")
}
