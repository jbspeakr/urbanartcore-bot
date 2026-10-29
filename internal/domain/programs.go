package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Feature programs. Each posts one post about more than one archive post: a
// written text, up to MaxPostImages of their photos, and the link to the
// page that gathers them.
const (
	ProgramCollection = "collection"
	ProgramArtist     = "artist-spotlight"
	ProgramBackThen   = "back-then"
)

const (
	// SpotlightMinPosts is how many posts an artist needs for a spotlight.
	SpotlightMinPosts = 5
	// SpotlightRest is how long an artist rests after a spotlight.
	SpotlightRest = 182 * 24 * time.Hour
	// ArchiveURL is the site's archive by month, the link of a back-then post.
	ArchiveURL = "https://www.urbanartcore.com/archives/"
	// featureTeaserWords caps the teaser of each post in a feature's source.
	featureTeaserWords = 60
)

// ErrNothingToFeature means a program has no candidate for the day.
var ErrNothingToFeature = errors.New("nothing to feature")

// Feature is the material of one feature post, the same on every platform:
// what it is about, what the writer and the critic read, and the posts whose
// photos go with it.
type Feature struct {
	Program string
	URL     string // What the post links: the collection, the artist's tag page, the archive
	Title   string
	Subject string // What the post recommends, for the writer: "a collection of stories"
	Section string
	Intro   string
	Artists []string
	Lines   []string // The posts the feature covers, one line each; built from Picks when empty
	Picks   []*Post  // The posts whose cover photos go with the post, at most MaxPostImages
}

// CollectionFeature lays out a collection: its title and description, each
// item in reading order with the collection's note, and the photos of its
// first items. posts maps post URLs to the catalogue's posts.
func CollectionFeature(c Collection, posts map[string]*Post) (Feature, error) {
	if len(c.Items) == 0 {
		return Feature{}, fmt.Errorf("%w: collection %q has no items", ErrNothingToFeature, c.Slug)
	}
	f := Feature{
		Program: ProgramCollection,
		URL:     c.URL,
		Title:   c.Title,
		Subject: "a collection of stories, read in the order given",
		Section: "Collection",
		Intro:   joinLines(c.Description, c.Intro),
	}
	for i, item := range c.Items {
		line := fmt.Sprintf("%d. %s", i+1, item.Title)
		if !item.Date.IsZero() {
			line += " (" + item.Date.Format("January 2006") + ")"
		}
		if note := strings.TrimSpace(item.Note); note != "" {
			line += ": " + note
		}
		f.Lines = append(f.Lines, line)
		if post := posts[item.URL]; post != nil && len(f.Picks) < MaxPostImages {
			f.Picks = append(f.Picks, post)
		}
	}
	return f, nil
}

// ArtistFeature lays out an artist's spotlight: how long the archive
// followed them, and up to four of their posts spread from the first to the
// last.
func ArtistFeature(artist TagPage) (Feature, error) {
	if len(artist.Posts) < 3 {
		return Feature{}, fmt.Errorf("%w: %s has fewer than three posts", ErrNothingToFeature, artist.Name)
	}
	posts := artist.Posts
	first, last := posts[0], posts[len(posts)-1]
	years := fmt.Sprintf("in %d", first.Date().Year())
	if first.Date().Year() != last.Date().Year() {
		years = fmt.Sprintf("between %d and %d", first.Date().Year(), last.Date().Year())
	}
	return Feature{
		Program: ProgramArtist,
		URL:     artist.URL,
		Title:   artist.Name,
		Subject: "the archive's posts about an artist",
		Section: "Artist",
		Intro:   fmt.Sprintf("%d posts about %s, published %s. Below are %d of them, from the first to the last.", len(posts), artist.Name, years, min(len(posts), MaxPostImages)),
		Artists: []string{artist.Name},
		Picks:   spread(posts, MaxPostImages),
	}, nil
}

// BackThenFeature lays out the archive's month, year by year: the
// best-scoring post of that month in each year, at most MaxPostImages years.
func BackThenFeature(posts []*Post, month time.Month, date time.Time, lastFeatured map[string]time.Time) (Feature, error) {
	byYear := make(map[int][]Candidate)
	for _, post := range posts {
		if post == nil || post.Date().Month() != month {
			continue
		}
		c := ScoreOnThisDay(post, date, lastFeatured)
		byYear[post.Date().Year()] = append(byYear[post.Date().Year()], c)
	}
	years := make([]int, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Ints(years)

	f := Feature{
		Program: ProgramBackThen,
		URL:     ArchiveURL,
		Title:   month.String() + ", year by year",
		Subject: "a month of the archive, year by year",
		Section: "The archive by month",
		Intro:   fmt.Sprintf("One post from each %s the blog was running.", month),
	}
	for _, year := range years {
		if best, ok := Best(byYear[year], date, lastFeatured); ok && len(f.Picks) < MaxPostImages {
			f.Picks = append(f.Picks, best.Post)
		}
	}
	if len(f.Picks) < 2 {
		return Feature{}, fmt.Errorf("%w: fewer than two years with a post in %s", ErrNothingToFeature, month)
	}
	return f, nil
}

// spread picks n posts from posts, the first and the last among them and the
// rest evenly between.
func spread(posts []*Post, n int) []*Post {
	if len(posts) <= n {
		return append([]*Post(nil), posts...)
	}
	picks := make([]*Post, 0, n)
	for i := 0; i < n; i++ {
		picks = append(picks, posts[i*(len(posts)-1)/(n-1)])
	}
	return picks
}

// Period says when the feature's posts were published, for the writer:
// "from May 2010 to June 2012", or "in May 2010" for a single month.
func (f Feature) Period() string {
	var from, to time.Time
	for _, post := range f.Picks {
		if d := post.Date(); from.IsZero() || d.Before(from) {
			from = d
		}
		if d := post.Date(); d.After(to) {
			to = d
		}
	}
	switch {
	case from.IsZero():
		return ""
	case from.Year() == to.Year() && from.Month() == to.Month():
		return "in " + from.Format("January 2006")
	default:
		return fmt.Sprintf("from %s to %s", from.Format("January 2006"), to.Format("January 2006"))
	}
}

// Post is the feature as one post for the writer, the critic and the
// fallback template: the feature's URL, the date of its first pick, and an
// article whose text lists the posts it covers. Picks read in full give
// their teasers to the list.
func (f Feature) Post() (*Post, error) {
	date := time.Time{}
	for _, post := range f.Picks {
		if date.IsZero() || post.Date().Before(date) {
			date = post.Date()
		}
	}
	if date.IsZero() {
		return nil, fmt.Errorf("%w: %s has no posts to show", ErrNothingToFeature, f.Title)
	}
	post, err := NewPost(f.URL, date.Format(DateLayout))
	if err != nil {
		return nil, err
	}
	lines := f.Lines
	if len(lines) == 0 {
		for _, pick := range f.Picks {
			lines = append(lines, pickLine(pick))
		}
	}
	return post.WithArticle(Article{
		Title:       f.Title,
		Section:     f.Section,
		Description: f.Intro,
		Body:        strings.Join(lines, "\n"),
		Tags:        Tags{Artists: f.Artists},
	}), nil
}

// pickLine is one post of a feature: when it was published, its title, and
// its teaser.
func pickLine(post *Post) string {
	article := post.Article()
	title := article.Title
	if title == "" {
		title = post.URI()
	}
	line := fmt.Sprintf("%s: %s", post.Date().Format("January 2006"), title)
	if teaser := TruncateWords(article.Description, featureTeaserWords); teaser != "" {
		line += ". " + teaser
	}
	return line
}

// ChooseCollection returns the collection featured longest ago, or never.
// lastFeatured is keyed by collection URL.
func ChooseCollection(collections []Collection, date time.Time, lastFeatured map[string]time.Time) (Collection, bool) {
	var best *Collection
	for i := range collections {
		c := &collections[i]
		if len(c.Items) == 0 {
			continue
		}
		if best == nil || featuredBefore(c.URL, best.URL, date, lastFeatured) {
			best = c
		}
	}
	if best == nil {
		return Collection{}, false
	}
	return *best, true
}

// ChooseArtist returns the artist for a spotlight: at least
// SpotlightMinPosts posts, not spotlighted within SpotlightRest, the most
// posts first. lastFeatured is keyed by tag page URL.
func ChooseArtist(artists []TagPage, date time.Time, lastFeatured map[string]time.Time) (TagPage, bool) {
	var best *TagPage
	for i := range artists {
		a := &artists[i]
		if len(a.Posts) < SpotlightMinPosts {
			continue
		}
		if last, ok := lastFeatured[a.URL]; ok && date.Sub(last) < SpotlightRest {
			continue
		}
		if best == nil || len(a.Posts) > len(best.Posts) ||
			(len(a.Posts) == len(best.Posts) && featuredBefore(a.URL, best.URL, date, lastFeatured)) {
			best = a
		}
	}
	if best == nil {
		return TagPage{}, false
	}
	return *best, true
}

// featuredBefore reports whether a should go before b: never featured first,
// then featured longest ago, then a choice stable for the date.
func featuredBefore(a, b string, date time.Time, lastFeatured map[string]time.Time) bool {
	lastA, seenA := lastFeatured[a]
	lastB, seenB := lastFeatured[b]
	if seenA != seenB {
		return !seenA
	}
	if seenA && !lastA.Equal(lastB) {
		return lastA.Before(lastB)
	}
	day := date.Format(DateLayout)
	return tieBreak(day, a) < tieBreak(day, b)
}

func joinLines(lines ...string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" && line != "." {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
