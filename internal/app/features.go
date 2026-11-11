package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// publishFeature posts the slot's feature: it reads the posts whose photos
// go with it, and posts one post per
// platform through the same writer, critic and fallback as a single post. The
// ledger records it under the collection, tag page or archive URL.
func (ps *PublishService) publishFeature(ctx context.Context, req *PublishRequest, slot time.Time, history []domain.PublishRecord, feature domain.Feature) (*PublishResponse, error) {
	platforms, done, err := ps.targetPlatforms(req, slot, history)
	if done != nil || err != nil {
		return done, err
	}

	for i, pick := range feature.Picks {
		feature.Picks[i] = ps.readPost(ctx, pick)
	}
	post, err := feature.Post()
	if errors.Is(err, domain.ErrNothingToFeature) {
		slog.Default().Info("Rest day: the program has nothing to feature",
			slog.String("program", req.Program), slog.String("reason", err.Error()))
		return &PublishResponse{RestDay: true}, nil
	}
	if err != nil {
		return nil, ApplicationError{Err: fmt.Errorf("feature post: %w", err)}
	}
	slog.Default().Info("Feature planned",
		slog.String("program", feature.Program),
		slog.String("about", feature.Title),
		slog.String("url", feature.URL),
		slog.Int("posts", len(feature.Picks)))

	extra := map[string]string{ports.ContextSubject: feature.Subject}
	if period := feature.Period(); period != "" {
		extra[ports.ContextPublished] = "Its posts were published " + period + "."
	}
	return ps.publishPost(ctx, req, slot, platforms, postJob{
		post:    post,
		program: feature.Program,
		images:  ps.images(ctx, feature.Picks...),
		context: extra,
	})
}
