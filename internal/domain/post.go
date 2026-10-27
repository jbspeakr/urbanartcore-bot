package domain

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// DomainError represents business rule violations in the domain layer.
type DomainError struct {
	Err error // Underlying error
}

func (e DomainError) Error() string {
	return e.Err.Error()
}

func (e DomainError) Unwrap() error {
	return e.Err
}

// Common domain errors
var (
	ErrInvalidURI     = errors.New("invalid URI format")
	ErrInvalidDate    = errors.New("invalid date format")
	ErrDateOutOfRange = errors.New("date out of acceptable range")
)

const (
	// DateLayout defines the expected date format (YYYY-MM-DD)
	DateLayout = "2006-01-02"
	// MinValidYear defines the earliest acceptable year for posts
	MinValidYear = 2000
	// MaxValidYear defines the latest acceptable year for posts (current year + 1)
	MaxValidYear = 2030
)

// Post represents a blog post entity with validation and business rules.
// It encapsulates the core information needed to generate social media content
// for anniversary posts from Urban Art Core. Only URI and date are required;
// the article, what the site says about the post, is filled in when known.
type Post struct {
	uri     string    // Full URL to the blog post
	date    time.Time // Original publication date
	article Article   // What the site says about the post; zero when unknown
}

// Article holds what the site publishes about a post beyond its address and
// date. Every field may be empty: the catalogue snapshot only knows the URL,
// and old posts can lack a teaser, a cover or a credit.
type Article struct {
	Title       string // Headline of the post
	Description string // Teaser written for the post
	Section     string // Editorial section, for example "Street Art" or "Graffiti"
	Body        string // Body as plain text, without images, links or markup
	Tags        Tags   // Typed tags the site assigns
	Cover       Cover  // Lead photograph; zero when the post has none
	Credit      string // Photo credit for the post; empty when the photos are the author's
}

// Tags are a post's typed tags, by kind, as the site labels them.
type Tags struct {
	Artists    []string // Artists and crews, for example "Banksy"
	Places     []string // Cities and regions, for example "Berlin"
	Events     []string // Festivals and shows
	Techniques []string // For example "Stencil Art" or "Mural Art"
	Formats    []string // For example "Video" or "Interview"
	Topics     []string // Anything else the site tags the post with
}

// Cover is a post's lead photograph.
type Cover struct {
	URL    string // Absolute URL of the image
	Alt    string // The site's alt text, often only the post title
	Width  int    // Pixel width; zero when unknown
	Height int    // Pixel height; zero when unknown
}

// IsZero reports whether nothing is known about the article.
func (a Article) IsZero() bool {
	return a.Title == "" && a.Description == "" && a.Body == ""
}

// NewPost creates a new Post with validation of URI and date.
// It ensures URI format is valid and date is within acceptable range.
// Content extraction is delegated to Mistral AI via the URI.
func NewPost(uri, dateStr string) (*Post, error) {
	// Validate URI format
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, DomainError{Err: fmt.Errorf("%w: %v", ErrInvalidURI, err)}
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, DomainError{Err: fmt.Errorf("%w: missing scheme or host", ErrInvalidURI)}
	}

	// Validate and parse date
	parsedDate, err := time.Parse(DateLayout, dateStr)
	if err != nil {
		return nil, DomainError{Err: fmt.Errorf("%w: expected format YYYY-MM-DD", ErrInvalidDate)}
	}

	// Validate date range
	year := parsedDate.Year()
	if year < MinValidYear || year > MaxValidYear {
		return nil, DomainError{Err: fmt.Errorf("%w: year must be between %d and %d", ErrDateOutOfRange, MinValidYear, MaxValidYear)}
	}

	return &Post{
		uri:  uri,
		date: parsedDate,
	}, nil
}

// URI returns the full URL to the blog post.
func (p *Post) URI() string {
	if p == nil {
		return ""
	}
	return p.uri
}

// Date returns the original publication date.
func (p *Post) Date() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.date
}

// Article returns what the site says about the post.
func (p *Post) Article() Article {
	if p == nil {
		return Article{}
	}
	return p.article
}

// WithArticle returns a copy of the post that carries article.
func (p *Post) WithArticle(article Article) *Post {
	if p == nil {
		return nil
	}
	return &Post{uri: p.uri, date: p.date, article: article}
}

// DateString returns the date formatted as YYYY-MM-DD.
func (p *Post) DateString() string {
	if p == nil {
		return ""
	}
	return p.date.Format(DateLayout)
}

// AnniversaryDateKey returns the MM-DD formatted key used for grouping
// anniversary posts by date.
func (p *Post) AnniversaryDateKey() string {
	if p == nil {
		return ""
	}
	return p.date.Format("01-02")
}

// YearsAgo calculates how many years have passed since the original
// publication date, relative to the given reference date.
func (p *Post) YearsAgo(referenceDate time.Time) int {
	if p == nil {
		return 0
	}
	years := referenceDate.Year() - p.date.Year()

	// Adjust if the anniversary hasn't occurred yet this year
	// Compare month and day to handle leap years correctly
	refMonth, refDay := referenceDate.Month(), referenceDate.Day()
	postMonth, postDay := p.date.Month(), p.date.Day()

	if refMonth < postMonth || (refMonth == postMonth && refDay < postDay) {
		years--
	}

	return years
}

// IsAnniversary checks if the given date represents an anniversary
// of this post (same month and day, different year).
func (p *Post) IsAnniversary(checkDate time.Time) bool {
	if p == nil {
		return false
	}
	return p.date.Month() == checkDate.Month() && p.date.Day() == checkDate.Day()
}

// String returns a human-readable representation of the post.
func (p *Post) String() string {
	if p == nil {
		return "Post{nil}"
	}
	return fmt.Sprintf("Post{URI: %s, Date: %s}", p.uri, p.DateString())
}
