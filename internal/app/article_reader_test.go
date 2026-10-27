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

// stubArticleReader returns a fixed article or error and counts calls.
type stubArticleReader struct {
	article *ports.Article
	err     error
	calls   int
}

func (s *stubArticleReader) ReadArticle(ctx context.Context, uri string) (*ports.Article, error) {
	s.calls++
	return s.article, s.err
}

// runDryRunWithReader publishes 10-27 to both platforms in dry-run mode
// and returns the generation requests the generator received.
func runDryRunWithReader(t *testing.T, reader ports.ArticleReader) []*ports.GenerationRequest {
	t.Helper()
	ctx := context.Background()

	repo := &MockPostRepository{}
	repo.On("GetPostForDate", ctx, "10-27").Return(createTestPost("https://www.urbanartcore.com/bomb-it-israel/", "2010-10-27"), nil)

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

	service := NewPublishService(repo, gen, publishers).WithArticleReader(reader)
	_, err := service.PublishAnniversaryPosts(ctx, &PublishRequest{DateKey: "10-27", Platforms: []string{"all"}, DryRun: true})
	require.NoError(t, err)
	require.Len(t, requests, 2)
	return requests
}

func TestPublishAnniversaryPosts_PassesArticleToGenerator(t *testing.T) {
	reader := &stubArticleReader{article: &ports.Article{
		Title:       "Bomb It: Israel",
		Description: "Bomb It goes to Tel Aviv.",
		Excerpt:     "Writers in Tel Aviv talk about the wall.",
	}}

	for _, req := range runDryRunWithReader(t, reader) {
		assert.Equal(t, "Bomb It: Israel", req.Context["post_title"])
		assert.Equal(t, "Bomb It goes to Tel Aviv.", req.Context["post_description"])
		assert.Equal(t, "Writers in Tel Aviv talk about the wall.", req.Context["post_excerpt"])
	}
	assert.Equal(t, 1, reader.calls, "the post is read once for all platforms")
}

func TestPublishAnniversaryPosts_UnreadableArticleFallsBack(t *testing.T) {
	reader := &stubArticleReader{err: errors.New("connection refused")}

	for _, req := range runDryRunWithReader(t, reader) {
		assert.NotContains(t, req.Context, "post_title")
	}
}
