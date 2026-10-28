package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Thread programs. Each posts a short thread: a first post that says what
// the thread is, then one reply per archive post with its link.
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
	// ArchiveURL is the site's archive by month, the link of a back-then thread.
	ArchiveURL = "https://www.urbanartcore.com/archives/"
)

// ErrNothingToFeature means a program has no candidate for the day.
var ErrNothingToFeature = errors.New("nothing to feature")

// Thread is the material of one thread, the same on every platform: the
// first post and the replies, each with the link it points to.
type Thread struct {
	Program  string
	URL      string // What the thread is about: the collection, the artist's tag page, the archive
	Title    string // For logs and the plan
	Root     ThreadPart
	Replies  []ThreadPart
	Hashtags []string // On the first post only
}

// ThreadPart is one post of a thread.
type ThreadPart struct {
	Text string
	Link string
}

// CollectionThread lays out a collection as a thread: its title and
// description, then each item in reading order with the collection's note.
func CollectionThread(c Collection) (Thread, error) {
	if len(c.Items) == 0 {
		return Thread{}, fmt.Errorf("%w: collection %q has no items", ErrNothingToFeature, c.Slug)
	}
	t := Thread{
		Program:  ProgramCollection,
		URL:      c.URL,
		Title:    c.Title,
		Root:     ThreadPart{Text: joinLines(c.Title+".", c.Description), Link: c.URL},
		Hashtags: []string{HashtagStreetArt, HashtagGraffiti},
	}
	for i, item := range c.Items {
		head := fmt.Sprintf("%d/%d %s", i+1, len(c.Items), item.Title)
		if !item.Date.IsZero() {
			head += ", " + item.Date.Format("January 2006")
		}
		t.Replies = append(t.Replies, ThreadPart{Text: joinLines(head, item.Note), Link: item.URL})
	}
	return t, nil
}

// ArtistThread lays out an artist's spotlight: how long the archive followed
// them, then their first post, one from the middle and their last.
func ArtistThread(artist TagPage) (Thread, error) {
	if len(artist.Posts) < 3 {
		return Thread{}, fmt.Errorf("%w: %s has fewer than three posts", ErrNothingToFeature, artist.Name)
	}
	posts := artist.Posts
	first, last := posts[0], posts[len(posts)-1]
	picks := []*Post{first, posts[len(posts)/2], last}
	years := fmt.Sprintf("in %d", first.Date().Year())
	if first.Date().Year() != last.Date().Year() {
		years = fmt.Sprintf("between %d and %d", first.Date().Year(), last.Date().Year())
	}
	t := Thread{
		Program: ProgramArtist,
		URL:     artist.URL,
		Title:   artist.Name,
		Root: ThreadPart{
			Text: fmt.Sprintf("%s in the Urban Art Core archive: %d posts %s. The first, one from the middle, and the last.", artist.Name, len(posts), years),
			Link: artist.URL,
		},
		Hashtags: uniqueHashtags(HashtagFromLabel(artist.Name), HashtagStreetArt, HashtagGraffiti),
	}
	for _, post := range picks {
		t.Replies = append(t.Replies, datedPart(post))
	}
	return t, nil
}

// BackThenThread lays out the archive's month, year by year: one post per
// year, the best-scoring of that month.
func BackThenThread(posts []*Post, month time.Month, date time.Time, lastFeatured map[string]time.Time) (Thread, error) {
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

	t := Thread{
		Program:  ProgramBackThen,
		URL:      ArchiveURL,
		Title:    month.String() + ", year by year",
		Root:     ThreadPart{Text: fmt.Sprintf("%s in the Urban Art Core archive, year by year.", month), Link: ArchiveURL},
		Hashtags: []string{HashtagStreetArt, HashtagGraffiti},
	}
	for _, year := range years {
		if best, ok := Best(byYear[year], date, lastFeatured); ok {
			t.Replies = append(t.Replies, datedPart(best.Post))
		}
	}
	if len(t.Replies) < 2 {
		return Thread{}, fmt.Errorf("%w: fewer than two years with a post in %s", ErrNothingToFeature, month)
	}
	return t, nil
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

// Contents renders the thread for platform: the first post with the
// hashtags, then the replies. Bluesky shows each link as a card, Mastodon
// gets it in the text. A text too long for the platform is cut at a word.
func (t Thread) Contents(platform string) ([]*GeneratedContent, error) {
	root, err := threadContent(t.Root, platform, t.Hashtags)
	if err != nil {
		return nil, err
	}
	contents := []*GeneratedContent{root}
	for _, part := range t.Replies {
		reply, err := threadContent(part, platform, nil)
		if err != nil {
			return nil, err
		}
		contents = append(contents, reply)
	}
	return contents, nil
}

func threadContent(part ThreadPart, platform string, hashtags []string) (*GeneratedContent, error) {
	text := part.Text
	for words := len(strings.Fields(text)); words > 0; words-- {
		candidate := text
		if words < len(strings.Fields(text)) {
			candidate = TruncateWords(text, words)
		}
		if platform == PlatformMastodon {
			candidate += "\n" + part.Link
		}
		content, err := NewGeneratedContent(candidate, platform, hashtags, "", part.Link)
		if err == nil {
			return content, nil
		}
		if !errors.Is(err, ErrTextTooLong) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("thread post for %s does not fit on %s", part.Link, platform)
}

func datedPart(post *Post) ThreadPart {
	title := post.Article().Title
	if title == "" {
		title = post.URI()
	}
	return ThreadPart{Text: fmt.Sprintf("%s: %s", post.Date().Format("January 2006"), title), Link: post.URI()}
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

func uniqueHashtags(tags ...string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" && tag != "#" && !seen[strings.ToLower(tag)] {
			seen[strings.ToLower(tag)] = true
			out = append(out, tag)
		}
	}
	if len(out) > MaxPostHashtags {
		out = out[:MaxPostHashtags]
	}
	return out
}
