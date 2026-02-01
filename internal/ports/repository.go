package ports

import (
	"context"
	"errors"
	"fmt"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
)

// RepositoryError represents data access layer errors.
type RepositoryError struct {
	Op  string // Operation that caused the error
	Err error  // Underlying error
}

func (e RepositoryError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("repository.%s: %v", e.Op, e.Err)
}

func (e RepositoryError) Unwrap() error {
	return e.Err
}

// Common repository errors
var (
	ErrPostNotFound       = errors.New("post not found")
	ErrDateNotFound       = errors.New("no posts found for date")
	ErrInvalidDateFormat  = errors.New("invalid date format")
	ErrDataSourceNotFound = errors.New("data source not found")
	ErrDataCorrupted      = errors.New("data source corrupted")
)

// PostRepository defines the contract for accessing post data.
// It abstracts the data layer to support different storage mechanisms
// (JSON files, databases, APIs) while maintaining consistent behavior.
type PostRepository interface {
	// GetPostsByDate retrieves all posts for a specific anniversary date.
	// The dateKey should be in MM-DD format (e.g., "09-14").
	// Returns an empty slice if no posts are found for the date.
	// Returns error only for infrastructure failures, not for missing data.
	GetPostsByDate(ctx context.Context, dateKey string) ([]*domain.Post, error)

	// GetDateGroup retrieves posts for a date organized as a domain.DateGroup.
	// This provides additional context and methods useful for AI content generation.
	// Returns ErrDateNotFound if no posts exist for the specified date.
	GetDateGroup(ctx context.Context, dateKey string) (*domain.DateGroup, error)

	// ListAvailableDates returns all MM-DD date keys that have associated posts.
	// Results are returned in chronological order (01-01, 01-02, ..., 12-31).
	// This is useful for validating date inputs and discovering available content.
	ListAvailableDates(ctx context.Context) ([]string, error)

	// GetPostCount returns the total number of posts across all dates.
	// This is primarily used for metrics and validation purposes.
	GetPostCount(ctx context.Context) (int, error)

	// ValidateDataSource performs health checks on the underlying data source.
	// This includes checking file existence, data format validation,
	// and basic integrity checks. Used during application startup.
	ValidateDataSource(ctx context.Context) error
}

// PostRepositoryStats provides metrics about the repository data.
type PostRepositoryStats struct {
	TotalPosts       int         `json:"total_posts"`
	DatesWithPosts   int         `json:"dates_with_posts"`
	YearRange        [2]int      `json:"year_range"`         // [min_year, max_year]
	PostsPerMonth    map[int]int `json:"posts_per_month"`    // month -> count
	LargestDateGroup string      `json:"largest_date_group"` // MM-DD with most posts
	DataSourceSize   int64       `json:"data_source_size"`   // bytes
	LastValidated    string      `json:"last_validated"`     // RFC3339 timestamp
}

// StatsProvider is an optional interface that repository implementations
// can implement to provide detailed metrics about the data.
type StatsProvider interface {
	// GetStats returns comprehensive statistics about the repository data.
	// This is useful for monitoring, debugging, and data analysis.
	GetStats(ctx context.Context) (*PostRepositoryStats, error)
}
