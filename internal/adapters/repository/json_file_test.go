package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeberg.org/brennenstuhl/uac-socializer/internal/infrastructure"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

func TestNewJSONFileRepository_ValidFile(t *testing.T) {
	logger := createTestLogger(t)
	sampleFile := filepath.Join("testdata", "posts_sample.json")

	repo, err := NewJSONFileRepository(sampleFile, logger)

	require.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, sampleFile, repo.filePath)
	assert.NotNil(t, repo.data)
	assert.Len(t, repo.data, 3) // 3 date keys in sample file
}

func TestNewJSONFileRepository_FileNotFound(t *testing.T) {
	logger := createTestLogger(t)
	nonExistentFile := filepath.Join("testdata", "nonexistent.json")

	repo, err := NewJSONFileRepository(nonExistentFile, logger)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataSourceNotFound)
}

func TestNewJSONFileRepository_EmptyFilePath(t *testing.T) {
	logger := createTestLogger(t)

	repo, err := NewJSONFileRepository("", logger)

	assert.Nil(t, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file path cannot be empty")
}

func TestNewJSONFileRepository_NilLogger(t *testing.T) {
	sampleFile := filepath.Join("testdata", "posts_sample.json")

	repo, err := NewJSONFileRepository(sampleFile, nil)

	assert.Nil(t, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
}

func TestNewJSONFileRepository_CorruptedJSON(t *testing.T) {
	logger := createTestLogger(t)
	corruptedFile := filepath.Join("testdata", "posts_corrupted.json")

	repo, err := NewJSONFileRepository(corruptedFile, logger)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataCorrupted)
}

func TestNewJSONFileRepository_InvalidData(t *testing.T) {
	logger := createTestLogger(t)
	invalidFile := filepath.Join("testdata", "posts_invalid_data.json")

	repo, err := NewJSONFileRepository(invalidFile, logger)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataCorrupted)
}

func TestGetPostsByDate_ValidDate(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		dateKey  string
		expected int
	}{
		{
			name:     "multiple posts",
			dateKey:  "09-14",
			expected: 2,
		},
		{
			name:     "single post",
			dateKey:  "02-14",
			expected: 1,
		},
		{
			name:     "three posts",
			dateKey:  "12-31",
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			posts, err := repo.GetPostsByDate(ctx, tt.dateKey)

			require.NoError(t, err)
			assert.Len(t, posts, tt.expected)

			// Verify all posts have correct anniversary date key
			for _, post := range posts {
				assert.Equal(t, tt.dateKey, post.AnniversaryDateKey())
				assert.NotEmpty(t, post.URI())
				assert.False(t, post.Date().IsZero())
			}
		})
	}
}

func TestGetPostsByDate_InvalidDate(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		dateKey string
	}{
		{"invalid month", "13-01"},
		{"invalid day", "01-32"},
		{"wrong format", "1-1"},
		{"no dash", "0101"},
		{"extra characters", "01-01-extra"},
		{"empty string", ""},
		{"only spaces", "   "},
		{"text", "january-first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			posts, err := repo.GetPostsByDate(ctx, tt.dateKey)

			assert.Nil(t, posts)
			require.Error(t, err)

			var repoErr ports.RepositoryError
			assert.ErrorAs(t, err, &repoErr)
			assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrInvalidDateFormat)
		})
	}
}

func TestGetPostsByDate_EmptyResults(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	posts, err := repo.GetPostsByDate(ctx, "01-01")

	require.NoError(t, err)
	assert.NotNil(t, posts)
	assert.Empty(t, posts)
}

func TestGetPostsByDate_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	posts, err := repo.GetPostsByDate(ctx, "09-14")

	assert.Nil(t, posts)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
}

func TestGetDateGroup_ValidDate(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	dateGroup, err := repo.GetDateGroup(ctx, "09-14")

	require.NoError(t, err)
	assert.NotNil(t, dateGroup)
	assert.Equal(t, "09-14", dateGroup.DateKey())
	assert.Equal(t, 2, dateGroup.Count())
	assert.False(t, dateGroup.IsEmpty())
}

func TestGetDateGroup_DateNotFound(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	dateGroup, err := repo.GetDateGroup(ctx, "01-01")

	assert.Nil(t, dateGroup)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDateNotFound)
}

func TestGetDateGroup_InvalidDate(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	dateGroup, err := repo.GetDateGroup(ctx, "invalid")

	assert.Nil(t, dateGroup)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrInvalidDateFormat)
}

func TestListAvailableDates(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	dates, err := repo.ListAvailableDates(ctx)

	require.NoError(t, err)
	assert.Len(t, dates, 3)

	// Verify dates are sorted chronologically
	expectedDates := []string{"02-14", "09-14", "12-31"}
	assert.Equal(t, expectedDates, dates)
}

func TestListAvailableDates_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dates, err := repo.ListAvailableDates(ctx)

	assert.Nil(t, dates)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
}

func TestGetPostCount(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	count, err := repo.GetPostCount(ctx)

	require.NoError(t, err)
	assert.Equal(t, 6, count) // 2 + 1 + 3 from sample data
}

func TestGetPostCount_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count, err := repo.GetPostCount(ctx)

	assert.Equal(t, 0, count)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
}

func TestValidateDataSource_Success(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	err := repo.ValidateDataSource(ctx)

	require.NoError(t, err)
	assert.NotNil(t, repo.stats)
	assert.Equal(t, 6, repo.stats.TotalPosts)
	assert.Equal(t, 3, repo.stats.DatesWithPosts)
}

func TestValidateDataSource_FileNotFound(t *testing.T) {
	logger := createTestLogger(t)
	repo := &JSONFileRepository{
		filePath: filepath.Join("testdata", "nonexistent.json"),
		logger:   logger,
		data:     make(postsData),
	}
	ctx := context.Background()

	err := repo.ValidateDataSource(ctx)

	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataSourceNotFound)
}

func TestValidateDataSource_NoDataLoaded(t *testing.T) {
	logger := createTestLogger(t)
	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo := &JSONFileRepository{
		filePath: sampleFile,
		logger:   logger,
		data:     nil, // No data loaded
	}
	ctx := context.Background()

	err := repo.ValidateDataSource(ctx)

	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataCorrupted)
}

func TestValidateDataSource_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repo.ValidateDataSource(ctx)

	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
}

func TestGetStats(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	stats, err := repo.GetStats(ctx)

	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 6, stats.TotalPosts)
	assert.Equal(t, 3, stats.DatesWithPosts)
	assert.Equal(t, [2]int{2009, 2022}, stats.YearRange)
	assert.Equal(t, "12-31", stats.LargestDateGroup) // Has 3 posts
	assert.Greater(t, stats.DataSourceSize, int64(0))
	assert.NotEmpty(t, stats.LastValidated)

	// Check posts per month
	assert.Equal(t, 1, stats.PostsPerMonth[2])  // February
	assert.Equal(t, 2, stats.PostsPerMonth[9])  // September
	assert.Equal(t, 3, stats.PostsPerMonth[12]) // December
}

func TestGetStats_Cached(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	// First call should calculate stats
	stats1, err := repo.GetStats(ctx)
	require.NoError(t, err)

	// Second call should return cached stats
	stats2, err := repo.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, stats1.LastValidated, stats2.LastValidated)
}

func TestGetStats_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stats, err := repo.GetStats(ctx)

	assert.Nil(t, stats)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
}

func TestCompareDateKeys(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"same month, earlier day", "01-15", "01-20", true},
		{"same month, later day", "01-20", "01-15", false},
		{"earlier month", "01-15", "02-10", true},
		{"later month", "02-10", "01-15", false},
		{"december vs january", "12-31", "01-01", false},
		{"january vs december", "01-01", "12-31", true},
		{"same date", "01-01", "01-01", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareDateKeys(tt.a, tt.b)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestDateKeyRegexValidation(t *testing.T) {
	tests := []struct {
		name    string
		dateKey string
		valid   bool
	}{
		{"valid date", "01-15", true},
		{"valid leap day", "02-29", true},
		{"valid december", "12-31", true},
		{"invalid month zero", "00-15", false},
		{"invalid month 13", "13-15", false},
		{"invalid day zero", "01-00", false},
		{"invalid day 32", "01-32", false},
		{"single digit month", "1-15", false},
		{"single digit day", "01-5", false},
		{"no leading zeros", "1-5", false},
		{"empty string", "", false},
		{"wrong separator", "01/15", false},
		{"too many parts", "01-15-2023", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dateKeyRegex.MatchString(tt.dateKey)
			assert.Equal(t, tt.valid, result, "dateKey: %s", tt.dateKey)
		})
	}
}

// Helper functions

func createTestRepository(t *testing.T) *JSONFileRepository {
	t.Helper()
	logger := createTestLogger(t)
	sampleFile := filepath.Join("testdata", "posts_sample.json")

	repo, err := NewJSONFileRepository(sampleFile, logger)
	require.NoError(t, err)
	return repo
}

func createTestLogger(t *testing.T) *infrastructure.StructuredLogger {
	t.Helper()
	config := infrastructure.DefaultLoggerConfig()
	config.Level = infrastructure.LevelDebug

	logger, err := infrastructure.NewStructuredLogger(config)
	require.NoError(t, err)
	return logger
}

// Benchmark tests

func BenchmarkGetPostsByDate(b *testing.B) {
	logger, err := infrastructure.NewStructuredLogger(infrastructure.DefaultLoggerConfig())
	require.NoError(b, err)

	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo, err := NewJSONFileRepository(sampleFile, logger)
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetPostsByDate(ctx, "09-14")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListAvailableDates(b *testing.B) {
	logger, err := infrastructure.NewStructuredLogger(infrastructure.DefaultLoggerConfig())
	require.NoError(b, err)

	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo, err := NewJSONFileRepository(sampleFile, logger)
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.ListAvailableDates(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
