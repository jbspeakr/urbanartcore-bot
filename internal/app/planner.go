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

// dateKeyLayout formats a date as the catalogue's MM-DD key.
const dateKeyLayout = "01-02"

// Planner lays out the coming days: which program runs on each and what it
// features. It makes no model calls and publishes nothing.
type Planner struct {
	repository ports.PostRepository
	archive    ports.Archive // optional; without it every day is on this day
	ledger     ports.Ledger  // optional; without it nothing counts as featured before the plan starts

	// Read once per planner, on first use; collections only when a collection is due
	collections       []domain.Collection
	collectionsLoaded bool
	artists           []domain.TagPage
	allPosts          []*domain.Post
	loaded            bool
}

// NewPlanner creates a planner over the catalogue and, when not nil, the
// archive and the ledger.
func NewPlanner(repository ports.PostRepository, archive ports.Archive, ledger ports.Ledger) *Planner {
	return &Planner{repository: repository, archive: archive, ledger: ledger}
}

// Plan returns one slot per day for days days starting at from, following
// domain.Rhythm. Whatever one day features counts as featured for the days
// after it, so the plan never repeats a post, collection, artist or place
// within its rest period. A day the ledger already holds keeps what went out.
func (p *Planner) Plan(ctx context.Context, from time.Time, days int) ([]domain.Slot, error) {
	if p == nil || p.repository == nil {
		return nil, errors.New("planner needs a post repository")
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	var history []domain.PublishRecord
	if p.ledger != nil {
		var err error
		if history, err = p.ledger.Records(ctx, from.Add(-domain.RepeatWindow)); err != nil {
			return nil, fmt.Errorf("failed to read the ledger: %w", err)
		}
	}
	return p.plan(ctx, from, days, history, "")
}

// Slot returns the slot for date given the ledger history. An empty program
// follows domain.Rhythm; any other runs that program, or rests when it has
// nothing to feature.
func (p *Planner) Slot(ctx context.Context, date time.Time, history []domain.PublishRecord, program string) (domain.Slot, error) {
	slots, err := p.plan(ctx, date, 1, history, program)
	if err != nil {
		return domain.Slot{}, err
	}
	return slots[0], nil
}

func (p *Planner) plan(ctx context.Context, from time.Time, days int, history []domain.PublishRecord, program string) ([]domain.Slot, error) {
	if err := p.load(ctx); err != nil {
		return nil, err
	}
	lastFeatured := domain.LastFeatured(history)
	lastPlace := p.lastPlaces(history)
	published := make(map[string]domain.PublishRecord, len(history))
	for _, r := range history {
		published[r.Date] = r
	}

	slots := make([]domain.Slot, 0, days)
	for i := 0; i < days; i++ {
		date := from.AddDate(0, 0, i)
		var slot domain.Slot
		var err error
		if record, ok := published[date.Format(domain.DateLayout)]; ok {
			slot, err = p.publishedSlot(ctx, date, record, lastFeatured)
		} else {
			slot, err = p.slotFor(ctx, date, program, lastFeatured, lastPlace)
		}
		if err != nil {
			return nil, err
		}
		if !slot.IsRestDay() {
			lastFeatured[slot.Subject()] = date
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

// slotFor chooses what date features. A strong anniversary takes any day;
// otherwise the rhythm's program runs, falling back to on this day when it
// has nothing to feature. A forced program has no fallback.
func (p *Planner) slotFor(ctx context.Context, date time.Time, program string, lastFeatured, lastPlace map[string]time.Time) (domain.Slot, error) {
	onThisDay, err := p.onThisDay(ctx, date, lastFeatured)
	if err != nil {
		return domain.Slot{}, err
	}
	forced := program != ""
	if !forced {
		program = domain.Rhythm(date)
		if p.archive == nil {
			program = domain.ProgramOnThisDay
		}
	}
	if program == domain.ProgramOnThisDay {
		return onThisDay, nil
	}
	if !forced && !onThisDay.IsRestDay() {
		if artist, ok := domain.StrongAnniversary(onThisDay.Post, date, p.artistPosts()); ok {
			onThisDay.Reasons = append(onThisDay.Reasons, fmt.Sprintf("anniversary of a %s post instead of %s", artist, program))
			return onThisDay, nil
		}
	}

	slot, err := p.programSlot(ctx, date, program, lastFeatured, lastPlace)
	if errors.Is(err, domain.ErrNothingToFeature) {
		if forced {
			return domain.Slot{Date: date, Program: domain.ProgramRestDay, Reasons: []string{err.Error()}}, nil
		}
		onThisDay.Reasons = append(onThisDay.Reasons, program+" had nothing to feature")
		return onThisDay, nil
	}
	return slot, err
}

// programSlot runs one of the archive programs for date.
func (p *Planner) programSlot(ctx context.Context, date time.Time, program string, lastFeatured, lastPlace map[string]time.Time) (domain.Slot, error) {
	if p.archive == nil {
		return domain.Slot{}, fmt.Errorf("the %s program needs the archive", program)
	}
	slot := domain.Slot{Date: date, Program: program}
	var feature domain.Feature
	var err error
	switch program {
	case domain.ProgramCollection:
		collection, ok := domain.ChooseCollection(p.loadCollections(ctx), date, lastFeatured)
		if !ok {
			return slot, fmt.Errorf("%w: no collection with items", domain.ErrNothingToFeature)
		}
		feature, err = domain.CollectionFeature(collection, p.postsByURI())
		slot.Reasons = []string{fmt.Sprintf("%d posts", len(collection.Items))}
	case domain.ProgramArtist:
		artist, ok := domain.ChooseArtist(p.artists, date, lastFeatured)
		if !ok {
			return slot, fmt.Errorf("%w: no artist due for a spotlight", domain.ErrNothingToFeature)
		}
		feature, err = domain.ArtistFeature(artist)
		slot.Reasons = []string{fmt.Sprintf("%d posts", len(artist.Posts))}
	case domain.ProgramBackThen:
		feature, err = domain.BackThenFeature(p.allPosts, date.Month(), date, lastFeatured)
		slot.Reasons = []string{fmt.Sprintf("%d years", len(feature.Picks))}
	case domain.ProgramPlace:
		place, best, ok := domain.ChoosePlacePost(p.allPosts, date, lastFeatured, lastPlace)
		if !ok {
			return slot, fmt.Errorf("%w: no place due", domain.ErrNothingToFeature)
		}
		lastPlace[place] = date
		slot.Post, slot.Score, slot.Reasons = best.Post, best.Score, best.Reasons
		return slot, nil
	default:
		return slot, fmt.Errorf("unknown program %q", program)
	}
	if err != nil {
		return slot, err
	}
	if len(feature.Picks) == 0 {
		return slot, fmt.Errorf("%w: none of %s's posts is in the catalogue", domain.ErrNothingToFeature, feature.Title)
	}
	slot.Feature = &feature
	return slot, nil
}

// postsByURI maps the catalogue's post URLs to its posts.
func (p *Planner) postsByURI() map[string]*domain.Post {
	byURI := make(map[string]*domain.Post, len(p.allPosts))
	for _, post := range p.allPosts {
		byURI[post.URI()] = post
	}
	return byURI
}

// artistPosts counts the archive's posts per artist.
func (p *Planner) artistPosts() map[string]int {
	counts := make(map[string]int, len(p.artists))
	for _, a := range p.artists {
		counts[a.Name] = len(a.Posts)
	}
	return counts
}

// onThisDay scores the date's anniversary posts and returns the best one,
// or a rest day.
func (p *Planner) onThisDay(ctx context.Context, date time.Time, lastFeatured map[string]time.Time) (domain.Slot, error) {
	posts, err := p.repository.GetPostsForDate(ctx, date.Format(dateKeyLayout))
	if err != nil && !errors.Is(err, ports.ErrDateNotFound) {
		return domain.Slot{}, fmt.Errorf("failed to read posts for %s: %w", date.Format(domain.DateLayout), err)
	}
	return chooseOnThisDay(posts, date, lastFeatured), nil
}

// publishedSlot rebuilds the slot of a day the ledger already holds, so a
// rerun completes the missing platforms with the same post or feature.
func (p *Planner) publishedSlot(ctx context.Context, date time.Time, record domain.PublishRecord, lastFeatured map[string]time.Time) (domain.Slot, error) {
	slot := domain.Slot{Date: date, Program: record.Program, Reasons: []string{"already published"}}
	if slot.Program == "" {
		slot.Program = domain.ProgramOnThisDay
	}
	switch slot.Program {
	case domain.ProgramCollection:
		for _, c := range p.loadCollections(ctx) {
			if c.URL == record.PostURI {
				feature, err := domain.CollectionFeature(c, p.postsByURI())
				if err == nil {
					slot.Feature = &feature
				}
			}
		}
	case domain.ProgramArtist:
		for _, a := range p.artists {
			if a.URL == record.PostURI {
				feature, err := domain.ArtistFeature(a)
				if err == nil {
					slot.Feature = &feature
				}
			}
		}
	case domain.ProgramBackThen:
		// Rebuilt without the history, so the picks match what went out
		feature, err := domain.BackThenFeature(p.allPosts, date.Month(), date, nil)
		if err == nil {
			slot.Feature = &feature
		}
	default:
		posts, err := p.repository.GetPostsForDate(ctx, date.Format(dateKeyLayout))
		if err != nil && !errors.Is(err, ports.ErrDateNotFound) {
			return domain.Slot{}, err
		}
		slot.Post = postByURI(append(posts, p.allPosts...), record.PostURI)
	}
	if slot.IsRestDay() {
		// What went out is no longer in the catalogue; nothing can be completed
		slot.Reasons = []string{"already published, no longer in the catalogue"}
		slot.Program = record.Program
	}
	return slot, nil
}

// load reads the archive once.
func (p *Planner) load(ctx context.Context) error {
	if p.loaded || p.archive == nil {
		return nil
	}
	var err error
	if p.allPosts, err = p.archive.AllPosts(ctx); err != nil {
		return fmt.Errorf("failed to read the catalogue: %w", err)
	}
	if p.artists, err = p.archive.Artists(ctx); err != nil {
		return fmt.Errorf("failed to read the artists: %w", err)
	}
	p.loaded = true
	return nil
}

// loadCollections reads the collections once, when the first one is due.
// They take a request each; failing to read them costs the collection
// program, not the plan.
func (p *Planner) loadCollections(ctx context.Context) []domain.Collection {
	if p.collectionsLoaded || p.archive == nil {
		return p.collections
	}
	p.collectionsLoaded = true
	collections, err := p.archive.Collections(ctx)
	if err != nil {
		slog.Default().Warn("Could not read the collections", slog.String("error", err.Error()))
		return nil
	}
	p.collections = collections
	return collections
}

// lastPlaces maps each place featured as place of the week to the day it
// went out, from the places of the posts the ledger recorded.
func (p *Planner) lastPlaces(history []domain.PublishRecord) map[string]time.Time {
	last := make(map[string]time.Time)
	for _, r := range history {
		if r.Program != domain.ProgramPlace {
			continue
		}
		date, err := time.Parse(domain.DateLayout, r.Date)
		if err != nil {
			continue
		}
		if post := postByURI(p.allPosts, r.PostURI); post != nil {
			for _, place := range post.Article().Tags.Places {
				if prev, ok := last[place]; !ok || date.After(prev) {
					last[place] = date
				}
			}
		}
	}
	return last
}

// chooseOnThisDay scores the day's anniversary posts and returns the best
// one as the slot, or a rest day when none reaches domain.RestFloor.
func chooseOnThisDay(posts []*domain.Post, date time.Time, lastFeatured map[string]time.Time) domain.Slot {
	candidates := make([]domain.Candidate, 0, len(posts))
	for _, post := range posts {
		candidates = append(candidates, domain.ScoreOnThisDay(post, date, lastFeatured))
	}
	best, ok := domain.Best(candidates, date, lastFeatured)
	if !ok {
		reasons := []string{"no anniversary post"}
		if len(posts) > 0 {
			reasons = []string{fmt.Sprintf("all %d posts featured within a year", len(posts))}
		}
		return domain.Slot{Date: date, Program: domain.ProgramRestDay, Reasons: reasons}
	}
	return domain.Slot{Date: date, Program: domain.ProgramOnThisDay, Post: best.Post, Score: best.Score, Reasons: best.Reasons}
}

func postByURI(posts []*domain.Post, uri string) *domain.Post {
	for _, post := range posts {
		if post != nil && post.URI() == uri {
			return post
		}
	}
	return nil
}
