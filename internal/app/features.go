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

// publishFeature runs a feature program for the slot: it picks what to
// feature, reads the posts whose photos go with it, and posts one post per
// platform through the same writer, critic and fallback as a single post. The
// ledger records it under the collection, tag page or archive URL.
func (ps *PublishService) publishFeature(ctx context.Context, req *PublishRequest, slot time.Time, history []domain.PublishRecord) (*PublishResponse, error) {
	feature, err := ps.buildFeature(ctx, req.Program, slot, history)
	if errors.Is(err, domain.ErrNothingToFeature) {
		slog.Default().Info("Rest day: the program has nothing to feature",
			slog.String("program", req.Program), slog.String("reason", err.Error()))
		return &PublishResponse{RestDay: true}, nil
	}
	if err != nil {
		return nil, ApplicationError{Err: err}
	}

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

// buildFeature picks what the program features on slot. Collections and
// artists are keyed in the ledger by their page URL, so history tells which
// were featured when.
func (ps *PublishService) buildFeature(ctx context.Context, program string, slot time.Time, history []domain.PublishRecord) (domain.Feature, error) {
	if ps.archive == nil {
		return domain.Feature{}, fmt.Errorf("the %s program needs the archive", program)
	}
	lastFeatured := domain.LastFeatured(history)
	switch program {
	case domain.ProgramCollection:
		collections, err := ps.archive.Collections(ctx)
		if err != nil {
			return domain.Feature{}, fmt.Errorf("failed to read the collections: %w", err)
		}
		collection, ok := domain.ChooseCollection(collections, slot, lastFeatured)
		if !ok {
			return domain.Feature{}, fmt.Errorf("%w: no collection with items", domain.ErrNothingToFeature)
		}
		posts, err := ps.archive.AllPosts(ctx)
		if err != nil {
			return domain.Feature{}, fmt.Errorf("failed to read the catalogue: %w", err)
		}
		byURI := make(map[string]*domain.Post, len(posts))
		for _, post := range posts {
			byURI[post.URI()] = post
		}
		return domain.CollectionFeature(collection, byURI)
	case domain.ProgramArtist:
		artists, err := ps.archive.Artists(ctx)
		if err != nil {
			return domain.Feature{}, fmt.Errorf("failed to read the artists: %w", err)
		}
		artist, ok := domain.ChooseArtist(artists, slot, lastFeatured)
		if !ok {
			return domain.Feature{}, fmt.Errorf("%w: no artist due for a spotlight", domain.ErrNothingToFeature)
		}
		return domain.ArtistFeature(artist)
	case domain.ProgramBackThen:
		posts, err := ps.archive.AllPosts(ctx)
		if err != nil {
			return domain.Feature{}, fmt.Errorf("failed to read the catalogue: %w", err)
		}
		return domain.BackThenFeature(posts, slot.Month(), slot, lastFeatured)
	default:
		return domain.Feature{}, fmt.Errorf("unknown feature program %q", program)
	}
}
