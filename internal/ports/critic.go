package ports

import (
	"context"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// Critic checks a generated draft against the post it was written from:
// every factual statement must be backed by the source, and the text must
// keep to the voice rules. A second model reads what the writer produced,
// so a confident invention about a real artist is caught before it posts.
type Critic interface {
	// Review checks content against post's article and returns a verdict
	// per claim and a voice score.
	Review(ctx context.Context, post *domain.Post, content *domain.GeneratedContent) (*domain.Review, error)

	// TestConnection verifies the critic's model is still served.
	TestConnection(ctx context.Context) error
}

// ImageDescriber writes alt text for a post's photograph: what the image
// shows, for someone who cannot see it.
type ImageDescriber interface {
	// Describe returns a short description of the image at imageURL, which
	// belongs to post. It may name the artists and places the post is
	// tagged with, nothing else.
	Describe(ctx context.Context, post *domain.Post, imageURL string) (string, error)

	// TestConnection verifies the model is served and accepts images.
	TestConnection(ctx context.Context) error
}
