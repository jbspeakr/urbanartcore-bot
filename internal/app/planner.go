package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// dateKeyLayout formats a date as the catalogue's MM-DD key.
const dateKeyLayout = "01-02"

// Planner lays out the coming days: which program runs on each and which
// post it shows. It makes no model calls and publishes nothing.
type Planner struct {
	repository ports.PostRepository
	ledger     ports.Ledger // optional; without it nothing counts as featured before the plan starts
}

// NewPlanner creates a planner over the catalogue and, when not nil, the ledger.
func NewPlanner(repository ports.PostRepository, ledger ports.Ledger) *Planner {
	return &Planner{repository: repository, ledger: ledger}
}

// Plan returns one slot per day for days days starting at from. A post
// chosen for one day counts as featured for the days after it, so the plan
// never repeats a post. A day the ledger already holds a post for keeps it.
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
	lastFeatured := domain.LastFeatured(history)
	published := make(map[string]string, len(history))
	for _, r := range history {
		published[r.Date] = r.PostURI
	}

	slots := make([]domain.Slot, 0, days)
	for i := 0; i < days; i++ {
		date := from.AddDate(0, 0, i)
		posts, err := p.repository.GetPostsForDate(ctx, date.Format(dateKeyLayout))
		if err != nil && !errors.Is(err, ports.ErrDateNotFound) {
			return nil, fmt.Errorf("failed to read posts for %s: %w", date.Format(domain.DateLayout), err)
		}

		var slot domain.Slot
		if uri, ok := published[date.Format(domain.DateLayout)]; ok && postByURI(posts, uri) != nil {
			slot = domain.Slot{Date: date, Program: domain.ProgramOnThisDay, Post: postByURI(posts, uri), Reasons: []string{"already published"}}
		} else {
			slot = chooseOnThisDay(posts, date, lastFeatured)
		}
		if !slot.IsRestDay() {
			lastFeatured[slot.Post.URI()] = date
		}
		slots = append(slots, slot)
	}
	return slots, nil
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
