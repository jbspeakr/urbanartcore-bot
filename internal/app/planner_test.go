package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

func TestPlanner_Plan(t *testing.T) {
	artist := createTestPost("https://www.urbanartcore.com/blu-in-berlin/", "2010-09-15").WithArticle(domain.Article{
		Tags: domain.Tags{Artists: []string{"Blu"}},
	})
	plain := createTestPost("https://www.urbanartcore.com/a-wall/", "2011-09-15")
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", mock.Anything, "09-14").Return([]*domain.Post{thalys, marker}, nil)
	repo.On("GetPostsForDate", mock.Anything, "09-15").Return([]*domain.Post{plain, artist}, nil)
	repo.On("GetPostsForDate", mock.Anything, "09-16").Return(nil, ports.RepositoryError{Err: ports.ErrDateNotFound})
	ledger := &memLedger{records: []domain.PublishRecord{
		{Date: "2026-09-14", Platform: domain.PlatformBluesky, PostURI: marker.URI()},
	}}

	slots, err := NewPlanner(repo, ledger).Plan(context.Background(), time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC), 3)
	require.NoError(t, err)
	require.Len(t, slots, 3)

	assert.Equal(t, "2026-09-14", slots[0].Date.Format(domain.DateLayout))
	assert.Equal(t, marker.URI(), slots[0].Post.URI(), "a day already published keeps its post")
	assert.Equal(t, []string{"already published"}, slots[0].Reasons)

	assert.Equal(t, artist.URI(), slots[1].Post.URI(), "the best-scoring post wins")
	assert.Equal(t, domain.ProgramOnThisDay, slots[1].Program)
	assert.Equal(t, 3.0, slots[1].Score)

	assert.True(t, slots[2].IsRestDay())
	assert.Equal(t, domain.ProgramRestDay, slots[2].Program)
	assert.Equal(t, []string{"no anniversary post"}, slots[2].Reasons)
	assert.Equal(t, "2025-09-14", ledger.since.Format(domain.DateLayout))
}

func TestPlanner_NeverRepeatsWithinThePlan(t *testing.T) {
	repo := &MockPostRepository{}
	// The same single post answers every day, as if it were each day's only candidate
	repo.On("GetPostsForDate", mock.Anything, mock.Anything).Return([]*domain.Post{thalys}, nil)

	slots, err := NewPlanner(repo, nil).Plan(context.Background(), ledgerNow, 3)
	require.NoError(t, err)
	assert.False(t, slots[0].IsRestDay())
	assert.True(t, slots[1].IsRestDay())
	assert.Equal(t, []string{"all 1 posts featured within a year"}, slots[1].Reasons)
	assert.True(t, slots[2].IsRestDay())
}

func TestPlanner_Errors(t *testing.T) {
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", mock.Anything, mock.Anything).Return(nil, errors.New("site down"))
	_, err := NewPlanner(repo, nil).Plan(context.Background(), ledgerNow, 1)
	assert.ErrorContains(t, err, "site down")

	_, err = NewPlanner(&MockPostRepository{}, &memLedger{readErr: errors.New("403")}).Plan(context.Background(), ledgerNow, 1)
	assert.ErrorContains(t, err, "failed to read the ledger")

	_, err = NewPlanner(nil, nil).Plan(context.Background(), ledgerNow, 1)
	assert.Error(t, err)
}
