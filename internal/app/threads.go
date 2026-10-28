package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// publishThread runs a thread program for the slot: it picks what to
// feature, renders the thread per platform and posts the first post, then
// each reply under the one before. The thread texts come from the site's
// own titles and collection notes, so no model writes them and no critic
// checks them.
func (ps *PublishService) publishThread(ctx context.Context, req *PublishRequest, slot time.Time, history []domain.PublishRecord) (*PublishResponse, error) {
	thread, err := ps.buildThread(ctx, req.Program, slot, history)
	if errors.Is(err, domain.ErrNothingToFeature) {
		slog.Default().Info("Rest day: the program has nothing to feature",
			slog.String("program", req.Program), slog.String("reason", err.Error()))
		return &PublishResponse{RestDay: true}, nil
	}
	if err != nil {
		return nil, ApplicationError{Err: err}
	}

	platforms, err := ps.resolvePlatforms(req.Platforms)
	if err != nil {
		return nil, ApplicationError{Err: err}
	}
	if len(platforms) == 0 {
		return nil, ApplicationError{Err: ErrNoPlatforms}
	}
	if platforms = ps.unpublished(platforms, slot, history); len(platforms) == 0 {
		return &PublishResponse{Published: true}, nil
	}

	subject, err := domain.NewPost(thread.URL, slot.Format(domain.DateLayout))
	if err != nil {
		return nil, ApplicationError{Err: fmt.Errorf("thread subject: %w", err)}
	}
	publishResult, err := domain.NewPublishResult(subject, req.DryRun)
	if err != nil {
		return nil, ApplicationError{Err: err}
	}
	response := &PublishResponse{
		Result:    publishResult,
		Generated: make(map[string]*domain.GeneratedContent),
		Threads:   make(map[string][]*domain.GeneratedContent),
	}
	slog.Default().Info("Thread planned",
		slog.String("program", thread.Program),
		slog.String("about", thread.Title),
		slog.String("url", thread.URL),
		slog.Int("replies", len(thread.Replies)))

	for _, platform := range platforms {
		contents, err := thread.Contents(platform)
		if err != nil {
			failed, _ := domain.NewPlatformResult(platform, domain.StatusFailed, nil)
			failed.SetFailure(fmt.Errorf("thread rendering failed: %w", err), 0)
			_ = publishResult.AddPlatformResult(failed)
			continue
		}
		response.Threads[platform] = contents
		response.Generated[platform] = contents[0]

		rootResult, err := ps.publishToPlatform(ctx, platform, contents[0], req.DryRun)
		if rootResult == nil {
			rootResult, _ = domain.NewPlatformResult(platform, domain.StatusFailed, contents[0])
			rootResult.SetFailure(err, 0)
		}
		_ = publishResult.AddPlatformResult(rootResult)
		if req.DryRun || err != nil || !rootResult.IsSuccess() {
			continue
		}
		ps.record(ctx, slot, thread.Program, thread.URL, rootResult)
		ps.publishReplies(ctx, platform, contents[1:], rootResult.RemoteRef())
	}

	publishResult.Complete()
	if publishResult.HasFailures() {
		response.Error = ErrPartialFailure
		if publishResult.SuccessCount() == 0 && !req.DryRun {
			response.Error = ErrCriticalFailure
		}
	}
	return response, nil
}

// publishReplies posts each reply under the one before it. A reply that
// fails ends the thread there: the first post is out and recorded, and a
// rerun would only post it again.
func (ps *PublishService) publishReplies(ctx context.Context, platform string, replies []*domain.GeneratedContent, rootRef string) {
	parent := rootRef
	for i, reply := range replies {
		if parent == "" {
			slog.Default().Warn("Thread stopped: the platform returned no reference to reply to",
				slog.String("platform", platform), slog.Int("posted_replies", i))
			return
		}
		result, err := ps.publishToPlatform(ctx, platform, reply.InReplyTo(rootRef, parent), false)
		if err != nil || result == nil || !result.IsSuccess() {
			attrs := []any{slog.String("platform", platform), slog.Int("reply", i+1), slog.Int("of", len(replies))}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
			}
			slog.Default().Error("Thread stopped: a reply failed", attrs...)
			return
		}
		parent = result.RemoteRef()
	}
	slog.Default().Info("Thread published", slog.String("platform", platform), slog.Int("replies", len(replies)))
}

// buildThread picks what the program features on slot. Collections and
// artists are keyed in the ledger by their page URL, so history tells which
// were featured when.
func (ps *PublishService) buildThread(ctx context.Context, program string, slot time.Time, history []domain.PublishRecord) (domain.Thread, error) {
	if ps.archive == nil {
		return domain.Thread{}, fmt.Errorf("the %s program needs the archive", program)
	}
	lastFeatured := domain.LastFeatured(history)
	switch program {
	case domain.ProgramCollection:
		collections, err := ps.archive.Collections(ctx)
		if err != nil {
			return domain.Thread{}, fmt.Errorf("failed to read the collections: %w", err)
		}
		collection, ok := domain.ChooseCollection(collections, slot, lastFeatured)
		if !ok {
			return domain.Thread{}, fmt.Errorf("%w: no collection with items", domain.ErrNothingToFeature)
		}
		return domain.CollectionThread(collection)
	case domain.ProgramArtist:
		artists, err := ps.archive.Artists(ctx)
		if err != nil {
			return domain.Thread{}, fmt.Errorf("failed to read the artists: %w", err)
		}
		artist, ok := domain.ChooseArtist(artists, slot, lastFeatured)
		if !ok {
			return domain.Thread{}, fmt.Errorf("%w: no artist due for a spotlight", domain.ErrNothingToFeature)
		}
		return domain.ArtistThread(artist)
	case domain.ProgramBackThen:
		posts, err := ps.archive.AllPosts(ctx)
		if err != nil {
			return domain.Thread{}, fmt.Errorf("failed to read the catalogue: %w", err)
		}
		return domain.BackThenThread(posts, slot.Month(), slot, lastFeatured)
	default:
		return domain.Thread{}, fmt.Errorf("unknown thread program %q", program)
	}
}
