package ports

import (
	"context"
	"errors"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// RepositoryError represents data access layer errors.
type RepositoryError struct {
	Err error // Underlying error
}

func (e RepositoryError) Error() string {
	return e.Err.Error()
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
	// GetPostForDate retrieves a single post for a specific anniversary date.
	// The dateKey should be in MM-DD format (e.g., "09-14").
	// If multiple posts exist for the date, one is randomly selected.
	// Returns ErrDateNotFound if no posts exist for the specified date.
	GetPostForDate(ctx context.Context, dateKey string) (*domain.Post, error)

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
