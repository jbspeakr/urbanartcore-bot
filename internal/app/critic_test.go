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

// sequenceGenerator returns drafts in turn and records the feedback each
// request carried.
type sequenceGenerator struct {
	ports.ContentGenerator
	drafts   []string
	err      error
	feedback []string
}

func (g *sequenceGenerator) GenerateContent(ctx context.Context, req *ports.GenerationRequest) (*domain.GeneratedContent, error) {
	g.feedback = append(g.feedback, req.Context[ports.ContextCriticFeedback])
	if g.err != nil && len(g.feedback) > 1 {
		return nil, g.err
	}
	text := g.drafts[min(len(g.feedback), len(g.drafts))-1]
	content, err := domain.NewGeneratedContent(text, req.Platform, req.Posts[0].Hashtags(), "prompt", req.Posts[0].URI())
	if err != nil {
		return nil, err
	}
	return content.WithClaims([]string{text}), nil
}

// sequenceCritic returns reviews in turn, repeating the last.
type sequenceCritic struct {
	reviews  []domain.Review
	err      error
	reviewed []string
}

func (c *sequenceCritic) Review(ctx context.Context, post *domain.Post, content *domain.GeneratedContent) (*domain.Review, error) {
	c.reviewed = append(c.reviewed, content.Text())
	if c.err != nil {
		return nil, c.err
	}
	review := c.reviews[min(len(c.reviewed), len(c.reviews))-1]
	return &review, nil
}

func (c *sequenceCritic) TestConnection(ctx context.Context) error { return nil }

var (
	approved = domain.Review{VoiceScore: domain.VoiceScoreGood, Verdicts: []domain.ClaimVerdict{{Claim: "ok", Supported: true}}}
	rejected = domain.Review{VoiceScore: domain.VoiceScoreGood, Verdicts: []domain.ClaimVerdict{{Claim: "The wall is red.", Supported: false}}}
)

// runWithCritic generates Bluesky content for the Banksy post in dry-run mode.
func runWithCritic(t *testing.T, gen *sequenceGenerator, critic ports.Critic) *domain.GeneratedContent {
	t.Helper()
	ctx := context.Background()
	post := createTestPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14").WithArticle(domain.Article{
		Title: "Banksy Revealed!",
		Tags:  domain.Tags{Artists: []string{"Banksy"}, Places: []string{"Berlin"}},
	})
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", ctx, "09-14").Return([]*domain.Post{post}, nil)
	pub := NewMockSocialPublisher(domain.PlatformBluesky)
	pub.On("ValidateContent", ctx, mock.Anything).Return(nil)

	service := NewPublishService(repo, gen, map[string]ports.SocialPublisher{domain.PlatformBluesky: pub}).WithCritic(critic)
	resp, err := service.PublishAnniversaryPosts(ctx, &PublishRequest{DateKey: "09-14", Platforms: []string{"bluesky"}, DryRun: true})
	require.NoError(t, err)
	require.Contains(t, resp.Generated, domain.PlatformBluesky)
	return resp.Generated[domain.PlatformBluesky]
}

func TestCritic_ApprovedDraftIsPublished(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"Banksy painted a wall in Berlin."}}
	critic := &sequenceCritic{reviews: []domain.Review{approved}}

	content := runWithCritic(t, gen, critic)
	assert.Equal(t, "Banksy painted a wall in Berlin.", content.Text())
	assert.False(t, content.IsFallback())
	assert.Equal(t, []string{""}, gen.feedback, "one draft, no feedback")
}

func TestCritic_RejectedDraftIsRewrittenWithFeedback(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"A red wall in Berlin.", "Banksy painted a wall in Berlin."}}
	critic := &sequenceCritic{reviews: []domain.Review{rejected, approved}}

	content := runWithCritic(t, gen, critic)
	assert.Equal(t, "Banksy painted a wall in Berlin.", content.Text())
	assert.False(t, content.IsFallback())
	require.Len(t, gen.feedback, 2)
	assert.Empty(t, gen.feedback[0])
	assert.Equal(t, `- Not supported by THE STORY: "The wall is red."`, gen.feedback[1])
	assert.Equal(t, []string{"A red wall in Berlin.", "Banksy painted a wall in Berlin."}, critic.reviewed)
}

func TestCritic_FallbackAfterMaxRewrites(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"A red wall in Berlin."}}
	critic := &sequenceCritic{reviews: []domain.Review{rejected}}

	content := runWithCritic(t, gen, critic)
	assert.True(t, content.IsFallback())
	assert.Equal(t, "Banksy Revealed!\nBanksy in Berlin, September 2011.", content.Text())
	assert.Len(t, gen.feedback, 1+MaxRewrites)
	assert.Len(t, critic.reviewed, 1+MaxRewrites)
}

func TestCritic_FallbackWhenCriticFails(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"Banksy painted a wall in Berlin."}}
	critic := &sequenceCritic{err: errors.New("critic down")}

	content := runWithCritic(t, gen, critic)
	assert.True(t, content.IsFallback(), "an unchecked draft never goes out")
}

func TestCritic_FallbackWhenRewriteFails(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"A red wall in Berlin."}, err: errors.New("rate limited")}
	critic := &sequenceCritic{reviews: []domain.Review{rejected}}

	content := runWithCritic(t, gen, critic)
	assert.True(t, content.IsFallback())
	assert.Len(t, gen.feedback, 2)
}

func TestCritic_NoCriticPublishesDraft(t *testing.T) {
	gen := &sequenceGenerator{drafts: []string{"A red wall in Berlin."}}

	content := runWithCritic(t, gen, nil)
	assert.Equal(t, "A red wall in Berlin.", content.Text())
}
