package domain

import (
	"net/url"
	"strings"
	"time"
)

// ProgramOnThisDay is the daily program: a post published on today's month
// and day in an earlier year.
const ProgramOnThisDay = "on-this-day"

// ProgramRestDay marks a day on which nothing is posted.
const ProgramRestDay = "rest-day"

// RepeatWindow is how long a post rests after it was featured before it is
// picked again. A day with no other candidate is a rest day.
const RepeatWindow = 365 * 24 * time.Hour

// PublishRecord is what the ledger keeps about one post that went out on
// one platform.
type PublishRecord struct {
	Key         string    // Unique per slot, post and platform, see LedgerKey
	Date        string    // Slot date, YYYY-MM-DD
	Program     string    // For example ProgramOnThisDay
	Platform    string    // bluesky or mastodon
	PostURI     string    // The blog post that was promoted
	RemoteURL   string    // The published status on the platform
	Text        string    // The text as posted, hashtags included
	Fallback    bool      // Whether the fallback template was posted
	PublishedAt time.Time // When the platform accepted it
}

// LedgerKey names a record, for example
// "2026-10-27/on-this-day/banksy-revealed/bluesky".
func LedgerKey(date time.Time, program, uri, platform string) string {
	return strings.Join([]string{date.Format(DateLayout), program, slugOf(uri), platform}, "/")
}

// slugOf returns the last path segment of a post URL.
func slugOf(uri string) string {
	path := uri
	if parsed, err := url.Parse(uri); err == nil {
		path = parsed.Path
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	return segments[len(segments)-1]
}
