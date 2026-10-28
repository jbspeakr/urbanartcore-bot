package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// stubPostReader returns the post with a fixed article, or an error, and counts calls.
type stubPostReader struct {
	article domain.Article
	err     error
	calls   int
}

func (s *stubPostReader) ReadPost(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return post.WithArticle(s.article), nil
}

// runDryRunWithReader publishes 10-27 to both platforms in dry-run mode
// and returns the generation requests the generator received.
func runDryRunWithReader(t *testing.T, reader ports.PostReader) []*ports.GenerationRequest {
	t.Helper()
	ctx := context.Background()

	catalogued := createTestPost("https://www.urbanartcore.com/bomb-it-israel/", "2010-10-27").
		WithArticle(domain.Article{Title: "Bomb It: Israel"})
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", ctx, "10-27").Return([]*domain.Post{catalogued}, nil)

	var requests []*ports.GenerationRequest
	gen := &MockContentGenerator{}
	gen.On("GenerateContent", ctx, mock.AnythingOfType("*ports.GenerationRequest")).
		Run(func(args mock.Arguments) { requests = append(requests, args.Get(1).(*ports.GenerationRequest)) }).
		Return(createTestGeneratedContent("Bomb It in Tel Aviv", "bluesky"), nil)

	publishers := map[string]ports.SocialPublisher{}
	for _, platform := range []string{domain.PlatformBluesky, domain.PlatformMastodon} {
		pub := NewMockSocialPublisher(platform)
		pub.On("ValidateContent", ctx, mock.Anything).Return(nil)
		publishers[platform] = pub
	}

	service := NewPublishService(repo, gen, publishers).WithPostReader(reader)
	_, err := service.PublishAnniversaryPosts(ctx, &PublishRequest{DateKey: "10-27", Platforms: []string{"all"}, DryRun: true})
	require.NoError(t, err)
	require.Len(t, requests, 2)
	return requests
}

func TestPublishAnniversaryPosts_PassesFullArticleToGenerator(t *testing.T) {
	reader := &stubPostReader{article: domain.Article{
		Title:       "Bomb It: Israel",
		Description: "Bomb It goes to Tel Aviv.",
		Body:        "Writers in Tel Aviv talk about the wall.",
		Tags:        domain.Tags{Places: []string{"Tel Aviv"}},
	}}

	for _, req := range runDryRunWithReader(t, reader) {
		require.Len(t, req.Posts, 1)
		article := req.Posts[0].Article()
		assert.Equal(t, "Writers in Tel Aviv talk about the wall.", article.Body)
		assert.Equal(t, []string{"Tel Aviv"}, article.Tags.Places)
	}
	assert.Equal(t, 1, reader.calls, "the post is read once for all platforms")
}

func TestPublishAnniversaryPosts_UnreadablePostKeepsCatalogueEntry(t *testing.T) {
	reader := &stubPostReader{err: errors.New("connection refused")}

	for _, req := range runDryRunWithReader(t, reader) {
		article := req.Posts[0].Article()
		assert.Equal(t, "Bomb It: Israel", article.Title, "the catalogue's title survives")
		assert.Empty(t, article.Body)
	}
}
