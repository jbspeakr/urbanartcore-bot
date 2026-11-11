package domain

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

// RestFloor is the lowest score a candidate needs to be posted. A day whose
// candidates all score below it is a rest day: an honest no beats a weak post.
const RestFloor = 1.0

// FormatRoundup is the format of the weekly link tips: a list of links to
// other sites' stories. They name artists and places from different links,
// so a post about one of them mixes up the others; they are never featured.
const FormatRoundup = "Link Tips"

// Slot is one calendar day of the plan: the program, and the post or feature
// chosen for it. A slot with neither is a rest day.
type Slot struct {
	Date    time.Time
	Program string
	Post    *Post    // Single-post programs: on this day, place of the week
	Feature *Feature // Feature programs
	Score   float64
	Reasons []string // What the choice is made of, for the plan output
}

// IsRestDay reports whether nothing is posted on the slot.
func (s Slot) IsRestDay() bool {
	return s.Post == nil && s.Feature == nil
}

// Subject is what the slot features: the post's URL, or the collection, tag
// page or archive a feature is about. The ledger records it.
func (s Slot) Subject() string {
	switch {
	case s.Post != nil:
		return s.Post.URI()
	case s.Feature != nil:
		return s.Feature.URL
	default:
		return ""
	}
}

// Title names what the slot features, for the plan output.
func (s Slot) Title() string {
	switch {
	case s.Post != nil:
		if title := s.Post.Article().Title; title != "" {
			return title
		}
		return s.Post.URI()
	case s.Feature != nil:
		return s.Feature.Title
	default:
		return ""
	}
}

// Candidate is a post scored for a slot.
type Candidate struct {
	Post    *Post
	Score   float64
	Reasons []string
}

// ScoreOnThisDay scores an anniversary candidate for the slot on date. A
// post featured within RepeatWindow before date scores zero; otherwise the
// score starts at RestFloor and rises for what makes a post worth showing: a
// named artist, a place, a summary that says something, and a round
// anniversary. A link roundup scores zero. lastFeatured maps post URIs to the day they last went out.
func ScoreOnThisDay(post *Post, date time.Time, lastFeatured map[string]time.Time) Candidate {
	c := Candidate{Post: post}
	if post == nil {
		return c
	}
	if last, ok := lastFeatured[post.URI()]; ok && date.Sub(last) < RepeatWindow {
		c.Reasons = []string{"featured " + last.Format(DateLayout)}
		return c
	}

	article := post.Article()
	for _, format := range article.Tags.Formats {
		if strings.EqualFold(format, FormatRoundup) {
			c.Reasons = []string{"link roundup"}
			return c
		}
	}

	c.Score = RestFloor
	if len(article.Tags.Artists) > 0 {
		c.Score += 2
		c.Reasons = append(c.Reasons, "artist")
	}
	if len(article.Tags.Places) > 0 {
		c.Score++
		c.Reasons = append(c.Reasons, "place")
	}
	if len(strings.Fields(article.Description)) >= 12 {
		c.Score++
		c.Reasons = append(c.Reasons, "summary")
	}
	if years := date.Year() - post.Date().Year(); years > 0 && years%5 == 0 {
		c.Score++
		c.Reasons = append(c.Reasons, fmt.Sprintf("%d years", years))
	}
	return c
}

// Best returns the highest-scoring candidate at or above RestFloor, and
// false when there is none. Ties go to the post that has never been
// featured, then to the one featured longest ago, then to a choice that is
// stable for the date but varies from day to day.
func Best(candidates []Candidate, date time.Time, lastFeatured map[string]time.Time) (Candidate, bool) {
	eligible := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Post != nil && c.Score >= RestFloor {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return Candidate{}, false
	}
	day := date.Format(DateLayout)
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		lastA, seenA := lastFeatured[a.Post.URI()]
		lastB, seenB := lastFeatured[b.Post.URI()]
		if seenA != seenB {
			return !seenA
		}
		if seenA && !lastA.Equal(lastB) {
			return lastA.Before(lastB)
		}
		return tieBreak(day, a.Post.URI()) < tieBreak(day, b.Post.URI())
	})
	return eligible[0], true
}

func tieBreak(day, uri string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(day + "|" + uri))
	return h.Sum64()
}

// LastFeatured maps each post URI in records to the latest slot date it went
// out on.
func LastFeatured(records []PublishRecord) map[string]time.Time {
	last := make(map[string]time.Time, len(records))
	for _, r := range records {
		date, err := time.Parse(DateLayout, r.Date)
		if err != nil {
			continue
		}
		if prev, ok := last[r.PostURI]; !ok || date.After(prev) {
			last[r.PostURI] = date
		}
	}
	return last
}
