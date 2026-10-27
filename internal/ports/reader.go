package ports

import (
	"context"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// PostReader reads a post's full article from its source, so generated posts
// can talk about what the post shows instead of guessing from its URL.
type PostReader interface {
	// ReadPost returns a copy of post that carries the full article: title,
	// teaser, body, typed tags, cover and photo credit.
	ReadPost(ctx context.Context, post *domain.Post) (*domain.Post, error)
}
