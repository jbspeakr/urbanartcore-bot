package ports

import (
	"context"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// Archive gives the editorial programs the whole archive: every post, the
// artists with their posts, and the site's collections.
type Archive interface {
	// AllPosts returns every post in the catalogue, oldest first.
	AllPosts(ctx context.Context) ([]*domain.Post, error)

	// Artists returns every artist tag with its posts, oldest first.
	Artists(ctx context.Context) ([]domain.TagPage, error)

	// Collections returns the site's collections with their items in reading order.
	Collections(ctx context.Context) ([]domain.Collection, error)
}
