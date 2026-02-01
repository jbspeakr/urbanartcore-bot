package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Common dategroup errors
var (
	ErrInvalidDateKey  = errors.New("invalid date key format")
	ErrEmptyDateGroup  = errors.New("date group cannot be empty")
	ErrDuplicatePost   = errors.New("duplicate post in date group")
	ErrDateKeyMismatch = errors.New("post date does not match group date key")
)

// DateKeyPattern is the regex pattern for validating MM-DD format
var DateKeyPattern = regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)

// DateGroup represents a collection of posts that share the same anniversary date.
// It provides a value object for grouping multiple posts by MM-DD date key
// and managing anniversary-related operations.
type DateGroup struct {
	dateKey string  // MM-DD format key (e.g., "09-14")
	posts   []*Post // Posts published on this date across different years
}

// NewDateGroup creates a new DateGroup with validation of the date key format.
// The date key must be in MM-DD format with zero-padding (e.g., "01-15", "12-31").
func NewDateGroup(dateKey string, posts []*Post) (*DateGroup, error) {
	const op = "NewDateGroup"

	// Validate date key format
	dateKey = strings.TrimSpace(dateKey)
	if !DateKeyPattern.MatchString(dateKey) {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: expected MM-DD format, got %q", ErrInvalidDateKey, dateKey)}
	}

	// Validate that we have at least one post
	if len(posts) == 0 {
		return nil, DomainError{Op: op, Err: ErrEmptyDateGroup}
	}

	// Validate that all posts match the date key and are unique
	seenURIs := make(map[string]bool)
	for i, post := range posts {
		if post == nil {
			return nil, DomainError{Op: op, Err: fmt.Errorf("post at index %d is nil", i)}
		}

		// Check that post's anniversary date matches the group key
		if post.AnniversaryDateKey() != dateKey {
			return nil, DomainError{Op: op, Err: fmt.Errorf("%w: post has key %s, group expects %s",
				ErrDateKeyMismatch, post.AnniversaryDateKey(), dateKey)}
		}

		// Check for duplicate posts by URI
		if seenURIs[post.URI()] {
			return nil, DomainError{Op: op, Err: fmt.Errorf("%w: URI %s", ErrDuplicatePost, post.URI())}
		}
		seenURIs[post.URI()] = true
	}

	// Create a copy of the posts slice to ensure immutability
	postsCopy := make([]*Post, len(posts))
	copy(postsCopy, posts)

	return &DateGroup{
		dateKey: dateKey,
		posts:   postsCopy,
	}, nil
}

// ParseDateKey creates a DateGroup from a MM-DD string without posts.
// This is useful for creating empty groups or validating date keys.
func ParseDateKey(dateKey string) (*DateGroup, error) {
	const op = "ParseDateKey"

	dateKey = strings.TrimSpace(dateKey)
	if !DateKeyPattern.MatchString(dateKey) {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: expected MM-DD format, got %q", ErrInvalidDateKey, dateKey)}
	}

	return &DateGroup{
		dateKey: dateKey,
		posts:   make([]*Post, 0),
	}, nil
}

// FromTime creates a DateGroup key from a time.Time value.
// This extracts the MM-DD portion and validates the format.
func FromTime(t time.Time) (*DateGroup, error) {
	dateKey := t.Format("01-02")
	return ParseDateKey(dateKey)
}

// DateKey returns the MM-DD formatted date key.
func (dg *DateGroup) DateKey() string {
	return dg.dateKey
}

// Posts returns a copy of the posts in this date group.
// The returned slice is safe to modify without affecting the original.
func (dg *DateGroup) Posts() []*Post {
	posts := make([]*Post, len(dg.posts))
	copy(posts, dg.posts)
	return posts
}

// Count returns the number of posts in this date group.
func (dg *DateGroup) Count() int {
	return len(dg.posts)
}

// IsEmpty returns true if the date group contains no posts.
func (dg *DateGroup) IsEmpty() bool {
	return len(dg.posts) == 0
}

// GetPost returns the post at the specified index, or nil if index is out of bounds.
func (dg *DateGroup) GetPost(index int) *Post {
	if index < 0 || index >= len(dg.posts) {
		return nil
	}
	return dg.posts[index]
}

// AddPost adds a new post to the date group with validation.
// Returns a new DateGroup instance (immutable operation).
func (dg *DateGroup) AddPost(post *Post) (*DateGroup, error) {
	const op = "AddPost"

	if post == nil {
		return nil, DomainError{Op: op, Err: errors.New("post cannot be nil")}
	}

	// Verify the post's date matches this group's key
	if post.AnniversaryDateKey() != dg.dateKey {
		return nil, DomainError{Op: op, Err: fmt.Errorf("%w: post has key %s, group expects %s",
			ErrDateKeyMismatch, post.AnniversaryDateKey(), dg.dateKey)}
	}

	// Check for duplicate URI
	for _, existingPost := range dg.posts {
		if existingPost.URI() == post.URI() {
			return nil, DomainError{Op: op, Err: fmt.Errorf("%w: URI %s", ErrDuplicatePost, post.URI())}
		}
	}

	// Create new posts slice with the additional post
	newPosts := make([]*Post, len(dg.posts)+1)
	copy(newPosts, dg.posts)
	newPosts[len(dg.posts)] = post

	return &DateGroup{
		dateKey: dg.dateKey,
		posts:   newPosts,
	}, nil
}

// MonthDay returns the month and day components as integers.
// Returns (month, day, error) where month is 1-12 and day is 1-31.
func (dg *DateGroup) MonthDay() (int, int, error) {
	parts := strings.Split(dg.dateKey, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid date key format: %s", dg.dateKey)
	}

	month, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid month in date key: %v", err)
	}

	day, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid day in date key: %v", err)
	}

	return month, day, nil
}

// ToTime converts the date key to a time.Time for the given year.
// This is useful for anniversary calculations and comparisons.
func (dg *DateGroup) ToTime(year int) (time.Time, error) {
	month, day, err := dg.MonthDay()
	if err != nil {
		return time.Time{}, err
	}

	// Validate the date exists in the given year
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Month() != time.Month(month) || date.Day() != day {
		return time.Time{}, fmt.Errorf("invalid date %s for year %d", dg.dateKey, year)
	}

	return date, nil
}

// IsToday checks if this date group's key matches today's date (MM-DD).
func (dg *DateGroup) IsToday() bool {
	today := time.Now().Format("01-02")
	return dg.dateKey == today
}

// IsAnniversaryOf checks if this date group represents an anniversary
// of the given time (same month and day).
func (dg *DateGroup) IsAnniversaryOf(t time.Time) bool {
	return dg.dateKey == t.Format("01-02")
}

// YearsRange returns the range of years covered by posts in this group.
// Returns (minYear, maxYear) or (0, 0) if the group is empty.
func (dg *DateGroup) YearsRange() (int, int) {
	if dg.IsEmpty() {
		return 0, 0
	}

	minYear := dg.posts[0].Date().Year()
	maxYear := minYear

	for _, post := range dg.posts[1:] {
		year := post.Date().Year()
		if year < minYear {
			minYear = year
		}
		if year > maxYear {
			maxYear = year
		}
	}

	return minYear, maxYear
}

// String returns a human-readable representation of the date group.
func (dg *DateGroup) String() string {
	return fmt.Sprintf("DateGroup{Key: %s, Posts: %d}", dg.dateKey, len(dg.posts))
}
