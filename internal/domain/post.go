package domain

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// DomainError represents business rule violations in the domain layer.
type DomainError struct {
	Op  string // Operation that caused the error
	Err error  // Underlying error
}

func (e DomainError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
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
// for anniversary posts from Urban Art Core. The URI is parsed by Mistral AI
// to extract content, so only URI and date are required.
type Post struct {
	uri  string    // Full URL to the blog post
	date time.Time // Original publication date
}

// NewPost creates a new Post with validation of URI and date.
// It ensures URI format is valid and date is within acceptable range.
// Content extraction is delegated to Mistral AI via the URI.
func NewPost(uri, dateStr string) (*Post, error) {
	const op = "NewPost"

	// Validate URI format
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: %v", ErrInvalidURI, err)}
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: missing scheme or host", ErrInvalidURI)}
	}

	// Validate and parse date
	parsedDate, err := time.Parse(DateLayout, dateStr)
	if err != nil {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: expected format YYYY-MM-DD", ErrInvalidDate)}
	}

	// Validate date range
	year := parsedDate.Year()
	if year < MinValidYear || year > MaxValidYear {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: year must be between %d and %d", ErrDateOutOfRange, MinValidYear, MaxValidYear)}
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
