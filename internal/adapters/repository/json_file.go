package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// rawPost represents the structure of posts as stored in the JSON file.
type rawPost struct {
	URI  string `json:"uri"`
	Date string `json:"date"`
}

// postsData represents the structure of the entire JSON file.
type postsData map[string][]*rawPost

// JSONFileRepository implements the PostRepository interface by reading
// post data from a JSON file. It loads the data once during initialization
// and provides O(1) lookups by date key.
type JSONFileRepository struct {
	filePath string    // Path to the JSON file
	data     postsData // In-memory cache of all post data
}

// dateKeyRegex validates MM-DD format with proper month/day ranges
var dateKeyRegex = regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)

// NewJSONFileRepository creates a new JSONFileRepository instance.
// It loads and validates the JSON file during initialization.
func NewJSONFileRepository(filePath string) (*JSONFileRepository, error) {

	if filePath == "" {
		return nil, ports.RepositoryError{

			Err: fmt.Errorf("file path cannot be empty"),
		}
	}

	repo := &JSONFileRepository{
		filePath: filePath,
	}

	// Load the JSON file during initialization
	if err := repo.loadData(); err != nil {
		return nil, ports.RepositoryError{Err: err}
	}

	slog.Default().Info("JSON file repository initialized",
		slog.String("file_path", filePath),
		slog.Int("total_dates", len(repo.data)),
	)

	return repo, nil
}

// loadData reads and parses the JSON file into memory.
func (r *JSONFileRepository) loadData() error {
	// Check file exists and is readable
	if _, err := os.Stat(r.filePath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ports.ErrDataSourceNotFound, r.filePath)
	}

	// Open and read the file
	file, err := os.Open(r.filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", r.filePath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Default().Error("failed to close file",
				slog.String("file", r.filePath),
				slog.Any("error", closeErr))
		}
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", r.filePath, err)
	}

	// Parse JSON
	var postsData postsData
	if err := json.Unmarshal(data, &postsData); err != nil {
		return fmt.Errorf("%w: invalid JSON format in %s: %v", ports.ErrDataCorrupted, r.filePath, err)
	}

	// Validate the data structure
	if err := r.validateData(postsData); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrDataCorrupted, err)
	}

	r.data = postsData
	return nil
}

// validateData performs basic validation on the loaded JSON data.
func (r *JSONFileRepository) validateData(data postsData) error {
	if len(data) == 0 {
		return fmt.Errorf("posts data is empty")
	}

	for dateKey, posts := range data {
		// Validate date key format
		if !dateKeyRegex.MatchString(dateKey) {
			return fmt.Errorf("invalid date key format: %s", dateKey)
		}

		// Validate posts array
		if posts == nil {
			return fmt.Errorf("posts array is nil for date key: %s", dateKey)
		}

		// Validate individual posts
		for i, post := range posts {
			if post == nil {
				return fmt.Errorf("post at index %d is nil for date key: %s", i, dateKey)
			}
			if post.URI == "" {
				return fmt.Errorf("post at index %d has empty URI for date key: %s", i, dateKey)
			}
			if post.Date == "" {
				return fmt.Errorf("post at index %d has empty date for date key: %s", i, dateKey)
			}

			// Validate date format and consistency
			parsedDate, err := time.Parse(domain.DateLayout, post.Date)
			if err != nil {
				return fmt.Errorf("post at index %d has invalid date format %s for date key %s: %v", i, post.Date, dateKey, err)
			}

			// Check that the post's anniversary date matches the key
			expectedKey := parsedDate.Format("01-02")
			if expectedKey != dateKey {
				return fmt.Errorf("post at index %d has date %s (key %s) but is stored under key %s", i, post.Date, expectedKey, dateKey)
			}
		}
	}

	return nil
}

// GetPostForDate retrieves a single post for a specific anniversary date.
// If multiple posts exist for the date, one is randomly selected.
func (r *JSONFileRepository) GetPostForDate(ctx context.Context, dateKey string) (*domain.Post, error) {

	// Validate date key format
	dateKey = strings.TrimSpace(dateKey)
	if !dateKeyRegex.MatchString(dateKey) {
		return nil, ports.RepositoryError{

			Err: fmt.Errorf("%w: expected MM-DD format, got %q", ports.ErrInvalidDateFormat, dateKey),
		}
	}

	slog.Default().Debug("Retrieving post for date",
		slog.String("date_key", dateKey),
	)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ports.RepositoryError{Err: ctx.Err()}
	default:
	}

	// Look up posts in the data map
	rawPosts, exists := r.data[dateKey]
	if !exists || len(rawPosts) == 0 {
		return nil, ports.RepositoryError{

			Err: fmt.Errorf("%w: %s", ports.ErrDateNotFound, dateKey),
		}
	}

	// If multiple posts, select one randomly
	selectedIndex := 0
	if len(rawPosts) > 1 {
		selectedIndex = rand.Intn(len(rawPosts))
		slog.Default().Debug("Multiple posts found, randomly selected",
			slog.String("date_key", dateKey),
			slog.Int("total_posts", len(rawPosts)),
			slog.Int("selected_index", selectedIndex),
		)
	}

	// Convert selected raw post to domain post
	selectedRawPost := rawPosts[selectedIndex]
	post, err := domain.NewPost(selectedRawPost.URI, selectedRawPost.Date)
	if err != nil {
		return nil, ports.RepositoryError{

			Err: fmt.Errorf("failed to create post at index %d for date %s: %w", selectedIndex, dateKey, err),
		}
	}

	slog.Default().Debug("Retrieved post successfully",
		slog.String("date_key", dateKey),
		slog.String("uri", post.URI()),
	)

	return post, nil
}

// ListAvailableDates returns all MM-DD date keys that have associated posts.
func (r *JSONFileRepository) ListAvailableDates(ctx context.Context) ([]string, error) {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ports.RepositoryError{Err: ctx.Err()}
	default:
	}

	dates := make([]string, 0, len(r.data))
	for dateKey := range r.data {
		dates = append(dates, dateKey)
	}

	// Sort dates in chronological order (MM-DD format)
	sort.Slice(dates, func(i, j int) bool {
		return compareDateKeys(dates[i], dates[j])
	})

	slog.Default().Debug("Listed available dates",
		slog.Int("total_dates", len(dates)),
	)

	return dates, nil
}

// compareDateKeys compares two MM-DD date keys for sorting.
func compareDateKeys(a, b string) bool {
	// Split and convert to integers for proper sorting
	aParts := strings.Split(a, "-")
	bParts := strings.Split(b, "-")

	if len(aParts) != 2 || len(bParts) != 2 {
		return a < b // fallback to string comparison
	}

	aMonth, _ := strconv.Atoi(aParts[0])
	aDay, _ := strconv.Atoi(aParts[1])
	bMonth, _ := strconv.Atoi(bParts[0])
	bDay, _ := strconv.Atoi(bParts[1])

	if aMonth != bMonth {
		return aMonth < bMonth
	}
	return aDay < bDay
}

// GetPostCount returns the total number of posts across all dates.
func (r *JSONFileRepository) GetPostCount(ctx context.Context) (int, error) {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return 0, ports.RepositoryError{Err: ctx.Err()}
	default:
	}

	totalPosts := 0
	for _, posts := range r.data {
		totalPosts += len(posts)
	}

	slog.Default().Debug("Counted total posts",
		slog.Int("total_posts", totalPosts),
	)

	return totalPosts, nil
}

// ValidateDataSource performs health checks on the underlying data source.
func (r *JSONFileRepository) ValidateDataSource(ctx context.Context) error {

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ports.RepositoryError{Err: ctx.Err()}
	default:
	}

	// Check file accessibility
	fileInfo, err := os.Stat(r.filePath)
	if os.IsNotExist(err) {
		return ports.RepositoryError{

			Err: fmt.Errorf("%w: %s", ports.ErrDataSourceNotFound, r.filePath),
		}
	}
	if err != nil {
		return ports.RepositoryError{

			Err: fmt.Errorf("failed to access file %s: %w", r.filePath, err),
		}
	}

	// Validate file is readable
	file, err := os.Open(r.filePath)
	if err != nil {
		return ports.RepositoryError{

			Err: fmt.Errorf("failed to open file %s: %w", r.filePath, err),
		}
	}
	if closeErr := file.Close(); closeErr != nil {
		slog.Default().Error("failed to close file during validation",
			slog.String("file", r.filePath),
			slog.Any("error", closeErr))
	}

	// Verify data integrity
	if len(r.data) == 0 {
		return ports.RepositoryError{

			Err: fmt.Errorf("%w: no data loaded", ports.ErrDataCorrupted),
		}
	}

	slog.Default().Info("Data source validation successful",
		slog.String("file_path", r.filePath),
		slog.Int64("file_size", fileInfo.Size()),
	)

	return nil
}
