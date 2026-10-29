package domain

import (
	"sort"
	"time"
)

// ProgramPlace is place of the week: one post from a place the archive
// covered often, chosen for the post rather than the date.
const ProgramPlace = "place-of-the-week"

const (
	// WellKnownArtistPosts is how many posts make an artist well known: a
	// round anniversary of a post naming one takes the slot from any program.
	WellKnownArtistPosts = 10
	// PlaceMinPosts is how many posts a place needs for place of the week.
	PlaceMinPosts = 10
	// PlaceRest is how long a place rests after it was featured.
	PlaceRest = 182 * 24 * time.Hour
)

// Rhythm is the program a date gets by default: the "back then" post on
// the first of the month; on Saturdays an artist spotlight or place of the
// week, alternating by week; a collection on the second and fourth
// Sunday, which never fall on the first; on this day otherwise.
func Rhythm(date time.Time) string {
	if date.Day() == 1 {
		return ProgramBackThen
	}
	switch date.Weekday() {
	case time.Saturday:
		if _, week := date.ISOWeek(); week%2 == 0 {
			return ProgramArtist
		}
		return ProgramPlace
	case time.Sunday:
		if day := date.Day(); (day >= 8 && day <= 14) || (day >= 22 && day <= 28) {
			return ProgramCollection
		}
	}
	return ProgramOnThisDay
}

// IsFeatureProgram reports whether program posts a feature: one post about several archive posts.
func IsFeatureProgram(program string) bool {
	switch program {
	case ProgramCollection, ProgramArtist, ProgramBackThen:
		return true
	}
	return false
}

// ChoosePlacePost picks place of the week: among places with at least
// PlaceMinPosts posts and not featured within PlaceRest, the one never or
// longest ago featured, more posts first on a tie; then that place's best
// post not featured within RepeatWindow. lastPlace maps place names to the
// day they were last featured.
func ChoosePlacePost(posts []*Post, date time.Time, lastFeatured, lastPlace map[string]time.Time) (string, Candidate, bool) {
	byPlace := make(map[string][]*Post)
	for _, post := range posts {
		if post == nil {
			continue
		}
		for _, place := range post.Article().Tags.Places {
			byPlace[place] = append(byPlace[place], post)
		}
	}
	places := make([]string, 0, len(byPlace))
	for place, list := range byPlace {
		if len(list) < PlaceMinPosts {
			continue
		}
		if last, ok := lastPlace[place]; ok && date.Sub(last) < PlaceRest {
			continue
		}
		places = append(places, place)
	}
	sort.Slice(places, func(i, j int) bool {
		a, b := places[i], places[j]
		lastA, seenA := lastPlace[a]
		lastB, seenB := lastPlace[b]
		if seenA != seenB {
			return !seenA
		}
		if seenA && !lastA.Equal(lastB) {
			return lastA.Before(lastB)
		}
		if len(byPlace[a]) != len(byPlace[b]) {
			return len(byPlace[a]) > len(byPlace[b])
		}
		return a < b
	})
	for _, place := range places {
		candidates := make([]Candidate, 0, len(byPlace[place]))
		for _, post := range byPlace[place] {
			candidates = append(candidates, ScoreOnThisDay(post, date, lastFeatured))
		}
		if best, ok := Best(candidates, date, lastFeatured); ok {
			best.Reasons = append([]string{"place " + place}, best.Reasons...)
			return place, best, true
		}
	}
	return "", Candidate{}, false
}

// StrongAnniversary reports whether post's anniversary on date beats the
// day's program: a round number of years (5, 10, 15) of a post naming an
// artist the archive covered at least WellKnownArtistPosts times. It
// returns that artist.
func StrongAnniversary(post *Post, date time.Time, artistPosts map[string]int) (string, bool) {
	if post == nil {
		return "", false
	}
	if years := date.Year() - post.Date().Year(); years <= 0 || years%5 != 0 {
		return "", false
	}
	for _, artist := range post.Article().Tags.Artists {
		if artistPosts[artist] >= WellKnownArtistPosts {
			return artist, true
		}
	}
	return "", false
}
