package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
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

// contextGenerator writes one draft and records what each request carried.
type contextGenerator struct {
	ports.ContentGenerator
	requests []*ports.GenerationRequest
}

func (g *contextGenerator) GenerateContent(ctx context.Context, req *ports.GenerationRequest) (*domain.GeneratedContent, error) {
	g.requests = append(g.requests, req)
	text := "A written post about " + req.Posts[0].Article().Title + "."
	if req.Platform == domain.PlatformMastodon {
		text += "\n" + req.Posts[0].URI()
	}
	return domain.NewGeneratedContent(text, req.Platform, req.Posts[0].Hashtags(), "prompt", req.Posts[0].URI())
}

func featureRig(t *testing.T, ledger *memLedger, archive *fakeArchive) (*ledgerRig, *contextGenerator) {
	t.Helper()
	rig := newLedgerRig(t, ledger)
	gen := &contextGenerator{}
	rig.service.generator = gen
	rig.service.WithArchive(archive)
	return rig, gen
}

func (r *ledgerRig) publishProgram(t *testing.T, program string, dryRun bool) *PublishResponse {
	t.Helper()
	resp, err := r.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{
		DateKey: "09-14", Platforms: []string{"all"}, DryRun: dryRun, Program: program,
	})
	require.NoError(t, err)
	return resp
}

// coveredPost is a catalogue post with a cover photograph.
func coveredPost(uri, date, title string) *domain.Post {
	return createTestPost(uri, date).WithArticle(domain.Article{
		Title: title,
		Cover: domain.Cover{URL: uri + "cover.jpg", Alt: title, Width: 1200, Height: 800},
	})
}

func TestFeature_CollectionIsOnePostWithPhotos(t *testing.T) {
	ledger := &memLedger{}
	archive := &fakeArchive{
		collections: []domain.Collection{stencilCollection},
		posts: []*domain.Post{
			coveredPost("https://www.urbanartcore.com/c215-paris/", "2010-03-02", "C215 in Paris"),
			coveredPost("https://www.urbanartcore.com/stencils-berlin/", "2010-06-12", "Stencils in Berlin"),
		},
	}
	rig, gen := featureRig(t, ledger, archive)

	resp := rig.publishProgram(t, domain.ProgramCollection, false)
	require.NoError(t, resp.Error)

	for _, platform := range []string{domain.PlatformBluesky, domain.PlatformMastodon} {
		assert.Equal(t, []string{stencilCollection.URL}, rig.publishers[platform].published, "one post, linking the collection")
		images := resp.Generated[platform].Images()
		require.Len(t, images, 2, "a photo from each item")
		assert.Equal(t, "https://www.urbanartcore.com/c215-paris/cover.jpg", images[0].URL)
	}

	require.NotEmpty(t, gen.requests)
	req := gen.requests[0]
	assert.Equal(t, "a collection of stories, read in the order given", req.Context[ports.ContextSubject])
	assert.Equal(t, "Its posts were published from March 2010 to June 2010.", req.Context[ports.ContextPublished])
	article := req.Posts[0].Article()
	assert.Equal(t, "A stencil art primer", article.Title)
	assert.Contains(t, article.Body, "1. C215 in Paris: Portraits on Paris doors.")

	require.Len(t, ledger.records, 2, "recorded once per platform")
	assert.Equal(t, domain.ProgramCollection, ledger.records[0].Program)
	assert.Equal(t, stencilCollection.URL, ledger.records[0].PostURI)
	assert.Equal(t, "2026-09-14/collection/stencil-art/"+ledger.records[0].Platform, ledger.records[0].Key)
}

func TestFeature_DryRunPostsNothing(t *testing.T) {
	ledger := &memLedger{}
	archive := &fakeArchive{
		collections: []domain.Collection{stencilCollection},
		posts:       []*domain.Post{coveredPost("https://www.urbanartcore.com/c215-paris/", "2010-03-02", "C215 in Paris")},
	}
	rig, _ := featureRig(t, ledger, archive)

	resp := rig.publishProgram(t, domain.ProgramCollection, true)
	assert.Contains(t, resp.Generated[domain.PlatformMastodon].Text(), "A written post about A stencil art primer.")
	assert.Empty(t, rig.publishers[domain.PlatformBluesky].published)
	assert.Empty(t, ledger.records)
}

func TestFeature_ArtistAndBackThen(t *testing.T) {
	var posts []*domain.Post
	for i, date := range []string{"2009-09-02", "2010-09-03", "2010-09-04", "2011-09-05", "2012-09-06"} {
		post := coveredPost("https://www.urbanartcore.com/roa-"+string(rune('a'+i))+"/", date, "Roa "+date)
		posts = append(posts, post.WithArticle(domain.Article{Title: post.Article().Title, Cover: post.Article().Cover, Tags: domain.Tags{Artists: []string{"Roa"}}}))
	}
	archive := &fakeArchive{
		artists: []domain.TagPage{{Name: "Roa", URL: "https://www.urbanartcore.com/roa/", Posts: posts}},
		posts:   posts,
	}

	rig, gen := featureRig(t, &memLedger{}, archive)
	resp := rig.publishProgram(t, domain.ProgramArtist, true)
	mastodon := resp.Generated[domain.PlatformMastodon]
	assert.Equal(t, "https://www.urbanartcore.com/roa/", mastodon.URI())
	assert.Contains(t, mastodon.Hashtags(), "#Roa")
	assert.Len(t, mastodon.Images(), 4, "first, last and two between")
	assert.Contains(t, gen.requests[0].Posts[0].Article().Description, "5 posts about Roa")

	rig, _ = featureRig(t, &memLedger{}, archive)
	resp = rig.publishProgram(t, domain.ProgramBackThen, true)
	bluesky := resp.Generated[domain.PlatformBluesky]
	assert.Equal(t, domain.ArchiveURL, bluesky.URI())
	assert.Len(t, bluesky.Images(), 4, "one photo for each of the four years")
}

func TestFeature_RestDayAndErrors(t *testing.T) {
	recent := &memLedger{records: []domain.PublishRecord{
		{Date: "2026-08-01", Platform: domain.PlatformBluesky, PostURI: "https://www.urbanartcore.com/roa/"},
	}}
	archive := &fakeArchive{artists: []domain.TagPage{{Name: "Roa", URL: "https://www.urbanartcore.com/roa/", Posts: make([]*domain.Post, 6)}}}
	rig, _ := featureRig(t, recent, archive)
	resp := rig.publishProgram(t, domain.ProgramArtist, false)
	assert.True(t, resp.RestDay, "no artist due for a spotlight")

	rig, _ = featureRig(t, &memLedger{}, &fakeArchive{collections: []domain.Collection{stencilCollection}})
	resp = rig.publishProgram(t, domain.ProgramCollection, false)
	assert.True(t, resp.RestDay, "no item of the collection is in the catalogue")

	rig, _ = featureRig(t, &memLedger{}, &fakeArchive{err: errors.New("site down")})
	_, err := rig.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: domain.ProgramCollection})
	assert.ErrorContains(t, err, "site down")

	_, err = rig.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: "weekly-quiz"})
	assert.ErrorContains(t, err, "unknown program")

	plain := newLedgerRig(t, &memLedger{})
	_, err = plain.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, Program: domain.ProgramCollection})
	assert.ErrorContains(t, err, "needs the archive")
}
