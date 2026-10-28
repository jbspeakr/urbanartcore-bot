package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// fakeArchive serves fixed collections, artists and posts.
type fakeArchive struct {
	collections []domain.Collection
	artists     []domain.TagPage
	posts       []*domain.Post
	err         error
}

func (a *fakeArchive) AllPosts(ctx context.Context) ([]*domain.Post, error) { return a.posts, a.err }
func (a *fakeArchive) Artists(ctx context.Context) ([]domain.TagPage, error) {
	return a.artists, a.err
}
func (a *fakeArchive) Collections(ctx context.Context) ([]domain.Collection, error) {
	return a.collections, a.err
}

var stencilCollection = domain.Collection{
	Slug:        "stencil-art",
	Title:       "A stencil art primer",
	Description: "From C215’s portraits to Berlin’s stencil scene.",
	URL:         "https://www.urbanartcore.com/collections/stencil-art/",
	Items: []domain.CollectionItem{
		{Position: 1, Title: "C215 in Paris", URL: "https://www.urbanartcore.com/c215-paris/", Note: "Portraits on Paris doors."},
		{Position: 2, Title: "Stencils in Berlin", URL: "https://www.urbanartcore.com/stencils-berlin/", Note: "Berlin’s stencil scene in 2010."},
	},
}

func threadRig(t *testing.T, ledger *memLedger, archive *fakeArchive) *ledgerRig {
	t.Helper()
	rig := newLedgerRig(t, ledger)
	rig.service.WithArchive(archive)
	return rig
}

func (r *ledgerRig) publishProgram(t *testing.T, program string, dryRun bool) *PublishResponse {
	t.Helper()
	resp, err := r.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{
		DateKey: "09-14", Platforms: []string{"all"}, DryRun: dryRun, Program: program,
	})
	require.NoError(t, err)
	return resp
}

func TestThread_CollectionPostsRootThenReplies(t *testing.T) {
	ledger := &memLedger{}
	rig := threadRig(t, ledger, &fakeArchive{collections: []domain.Collection{stencilCollection}})

	resp := rig.publishProgram(t, domain.ProgramCollection, false)
	require.NoError(t, resp.Error)

	mastodon := rig.publishers[domain.PlatformMastodon]
	assert.Equal(t, []string{stencilCollection.URL, "https://www.urbanartcore.com/c215-paris/", "https://www.urbanartcore.com/stencils-berlin/"}, mastodon.published)
	assert.Equal(t, []string{"", "ref:" + stencilCollection.URL, "ref:https://www.urbanartcore.com/c215-paris/"}, mastodon.parents,
		"each reply answers the post before it")

	require.Len(t, ledger.records, 2, "the first post is recorded once per platform")
	assert.Equal(t, domain.ProgramCollection, ledger.records[0].Program)
	assert.Equal(t, stencilCollection.URL, ledger.records[0].PostURI)
	assert.Equal(t, "2026-09-14/collection/stencil-art/"+ledger.records[0].Platform, ledger.records[0].Key)
}

func TestThread_DryRunShowsThreadAndPostsNothing(t *testing.T) {
	ledger := &memLedger{}
	rig := threadRig(t, ledger, &fakeArchive{collections: []domain.Collection{stencilCollection}})

	resp := rig.publishProgram(t, domain.ProgramCollection, true)
	require.Len(t, resp.Threads[domain.PlatformBluesky], 3)
	assert.Contains(t, resp.Threads[domain.PlatformMastodon][1].Text(), "Portraits on Paris doors.")
	assert.Empty(t, rig.publishers[domain.PlatformBluesky].published)
	assert.Empty(t, ledger.records)
}

func TestThread_FailedReplyStopsTheThread(t *testing.T) {
	ledger := &memLedger{}
	rig := threadRig(t, ledger, &fakeArchive{collections: []domain.Collection{stencilCollection}})
	rig.publishers[domain.PlatformBluesky].failAt = 2

	resp := rig.publishProgram(t, domain.ProgramCollection, false)
	assert.NoError(t, resp.Error, "the first post went out")
	assert.Len(t, rig.publishers[domain.PlatformBluesky].published, 1, "nothing after the failed reply")
	assert.Len(t, rig.publishers[domain.PlatformMastodon].published, 3)
	assert.Len(t, ledger.records, 2)
}

func TestThread_ArtistAndBackThen(t *testing.T) {
	var posts []*domain.Post
	for i, date := range []string{"2009-09-02", "2010-09-03", "2010-09-04", "2011-09-05", "2012-09-06"} {
		posts = append(posts, createTestPost("https://www.urbanartcore.com/roa-"+string(rune('a'+i))+"/", date).
			WithArticle(domain.Article{Title: "Roa " + date, Tags: domain.Tags{Artists: []string{"Roa"}}}))
	}
	archive := &fakeArchive{
		artists: []domain.TagPage{{Name: "Roa", URL: "https://www.urbanartcore.com/roa/", Posts: posts}},
		posts:   posts,
	}

	rig := threadRig(t, &memLedger{}, archive)
	resp := rig.publishProgram(t, domain.ProgramArtist, true)
	thread := resp.Threads[domain.PlatformMastodon]
	require.Len(t, thread, 4)
	assert.Contains(t, thread[0].Text(), "Roa in the Urban Art Core archive: 5 posts between 2009 and 2012.")
	assert.Contains(t, thread[0].Hashtags(), "#Roa")

	rig = threadRig(t, &memLedger{}, archive)
	resp = rig.publishProgram(t, domain.ProgramBackThen, true)
	thread = resp.Threads[domain.PlatformBluesky]
	require.Len(t, thread, 5, "root plus one post for each of the four years")
	assert.Contains(t, thread[0].Text(), "September in the Urban Art Core archive")
}

func TestThread_RestDayAndErrors(t *testing.T) {
	recent := &memLedger{records: []domain.PublishRecord{
		{Date: "2026-08-01", Platform: domain.PlatformBluesky, PostURI: "https://www.urbanartcore.com/roa/"},
	}}
	archive := &fakeArchive{artists: []domain.TagPage{{Name: "Roa", URL: "https://www.urbanartcore.com/roa/", Posts: make([]*domain.Post, 6)}}}
	resp := threadRig(t, recent, archive).publishProgram(t, domain.ProgramArtist, false)
	assert.True(t, resp.RestDay, "no artist due for a spotlight")

	rig := threadRig(t, &memLedger{}, &fakeArchive{err: errors.New("site down")})
	_, err := rig.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: domain.ProgramCollection})
	assert.ErrorContains(t, err, "site down")

	_, err = rig.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: "weekly-quiz"})
	assert.ErrorContains(t, err, "unknown program")

	plain := newLedgerRig(t, &memLedger{})
	_, err = plain.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: domain.ProgramCollection})
	assert.ErrorContains(t, err, "needs the archive")
}
