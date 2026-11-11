package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	slots, err := NewPlanner(repo, nil, ledger).Plan(context.Background(), time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC), 3)
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

	slots, err := NewPlanner(repo, nil, nil).Plan(context.Background(), ledgerNow, 3)
	require.NoError(t, err)
	assert.False(t, slots[0].IsRestDay())
	assert.True(t, slots[1].IsRestDay())
	assert.Equal(t, []string{"all 1 posts featured within a year"}, slots[1].Reasons)
	assert.True(t, slots[2].IsRestDay())
}

func TestPlanner_Errors(t *testing.T) {
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", mock.Anything, mock.Anything).Return(nil, errors.New("site down"))
	_, err := NewPlanner(repo, nil, nil).Plan(context.Background(), ledgerNow, 1)
	assert.ErrorContains(t, err, "site down")

	_, err = NewPlanner(&MockPostRepository{}, nil, &memLedger{readErr: errors.New("403")}).Plan(context.Background(), ledgerNow, 1)
	assert.ErrorContains(t, err, "failed to read the ledger")

	_, err = NewPlanner(nil, nil, nil).Plan(context.Background(), ledgerNow, 1)
	assert.Error(t, err)
}

// memRepository is a ports.PostRepository over posts in memory.
type memRepository struct{ byKey map[string][]*domain.Post }

func newMemRepository(posts []*domain.Post) *memRepository {
	r := &memRepository{byKey: map[string][]*domain.Post{}}
	for _, p := range posts {
		r.byKey[p.AnniversaryDateKey()] = append(r.byKey[p.AnniversaryDateKey()], p)
	}
	return r
}

func (r *memRepository) GetPostForDate(ctx context.Context, key string) (*domain.Post, error) {
	posts, err := r.GetPostsForDate(ctx, key)
	if err != nil {
		return nil, err
	}
	return posts[0], nil
}

func (r *memRepository) GetPostsForDate(ctx context.Context, key string) ([]*domain.Post, error) {
	if len(r.byKey[key]) == 0 {
		return nil, ports.RepositoryError{Err: ports.ErrDateNotFound}
	}
	return r.byKey[key], nil
}

func (r *memRepository) ListAvailableDates(ctx context.Context) ([]string, error) { return nil, nil }
func (r *memRepository) GetPostCount(ctx context.Context) (int, error)            { return 0, nil }
func (r *memRepository) ValidateDataSource(ctx context.Context) error             { return nil }

// archiveFixture builds an archive with a post on most days of November and
// December 2009, 2010 and 2012, four artists, two places and two collections.
// 2011 is left out: in 2026 its posts would be round anniversaries of
// well-known artists and take every slot.
func archiveFixture(t *testing.T) ([]*domain.Post, *fakeArchive) {
	t.Helper()
	artists := []string{"Blu", "Roa", "Herakut", "Nychos"}
	places := []string{"Berlin", "London"}
	var posts []*domain.Post
	pages := map[string]*domain.TagPage{}
	for year := 2009; year <= 2012; year++ {
		if year == 2011 {
			continue
		}
		for d := time.Date(year, 11, 1, 0, 0, 0, 0, time.UTC); d.Month() != 1; d = d.AddDate(0, 0, 1) {
			if d.Day()%9 == 0 && year != 2010 {
				continue // a few days without posts in most years
			}
			artist, place := artists[d.YearDay()%4], places[d.Day()%2]
			post := createTestPost(fmt.Sprintf("https://www.urbanartcore.com/p-%s/", d.Format("2006-01-02")), d.Format(domain.DateLayout)).
				WithArticle(domain.Article{Title: "Post " + d.Format("2 Jan 2006"), Tags: domain.Tags{Artists: []string{artist}, Places: []string{place}}})
			posts = append(posts, post)
			if pages[artist] == nil {
				pages[artist] = &domain.TagPage{Name: artist, URL: "https://www.urbanartcore.com/" + strings.ToLower(artist) + "/"}
			}
			pages[artist].Posts = append(pages[artist].Posts, post)
		}
	}
	item := func(post *domain.Post) domain.CollectionItem {
		return domain.CollectionItem{Title: post.Article().Title, URL: post.URI()}
	}
	archive := &fakeArchive{posts: posts, collections: []domain.Collection{
		{Slug: "stencil-art", Title: "A stencil art primer", URL: "https://www.urbanartcore.com/collections/stencil-art/",
			Items: []domain.CollectionItem{item(posts[0]), item(posts[1])}},
		{Slug: "bristol", Title: "One street, two summers", URL: "https://www.urbanartcore.com/collections/bristol/",
			Items: []domain.CollectionItem{item(posts[2])}},
	}}
	for _, name := range artists {
		archive.artists = append(archive.artists, *pages[name])
	}
	return posts, archive
}

// TestPlanner_ThirtyDays checks the phase gate on a fixture archive: every
// day covered, nothing repeated, features and place posts where the rhythm
// puts them.
func TestPlanner_ThirtyDays(t *testing.T) {
	posts, archive := archiveFixture(t)
	planner := NewPlanner(newMemRepository(posts), archive, nil)

	slots, err := planner.Plan(context.Background(), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 30)
	require.NoError(t, err)
	require.Len(t, slots, 30)

	subjects := map[string]string{}
	programs := map[string]int{}
	for _, s := range slots {
		date := s.Date.Format(domain.DateLayout)
		require.False(t, s.IsRestDay(), "%s is a rest day: %v", date, s.Reasons)
		if s.Program != domain.ProgramBackThen { // every back-then post links the archive
			prev, seen := subjects[s.Subject()]
			assert.False(t, seen, "%s repeats %s from %s", date, s.Subject(), prev)
			subjects[s.Subject()] = date
		}
		programs[s.Program]++
	}
	assert.Equal(t, domain.ProgramBackThen, slots[0].Program, "the first of the month")
	assert.Equal(t, domain.ProgramCollection, slots[7].Program, "the second Sunday")
	assert.Equal(t, domain.ProgramCollection, slots[21].Program, "the fourth Sunday")
	assert.Equal(t, domain.ProgramPlace, slots[6].Program, "Saturday of an odd week")
	assert.Equal(t, domain.ProgramArtist, slots[13].Program, "Saturday of an even week")
	assert.GreaterOrEqual(t, programs[domain.ProgramCollection]+programs[domain.ProgramArtist]+programs[domain.ProgramBackThen], 2)
	assert.Equal(t, 2, programs[domain.ProgramArtist], "four artists, none twice within half a year")
}

func TestPlanner_StrongAnniversaryTakesTheSlot(t *testing.T) {
	strong := createTestPost("https://www.urbanartcore.com/strong/", "2011-11-07").WithArticle(domain.Article{
		Tags: domain.Tags{Artists: []string{"Blu"}},
	})
	_, archive := archiveFixture(t) // Blu has well over ten posts there
	planner := NewPlanner(newMemRepository([]*domain.Post{strong}), archive, nil)

	slot, err := planner.Slot(context.Background(), time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), nil, "")
	require.NoError(t, err)
	assert.Equal(t, domain.ProgramOnThisDay, slot.Program, "15 years of a post naming a well-known artist beats place of the week")
	assert.Contains(t, slot.Reasons, "anniversary of a Blu post instead of place-of-the-week")
}
