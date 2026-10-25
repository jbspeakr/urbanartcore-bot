package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

func TestNewJSONFileRepository_ValidFile(t *testing.T) {
	sampleFile := filepath.Join("testdata", "posts_sample.json")

	repo, err := NewJSONFileRepository(sampleFile)

	require.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, sampleFile, repo.filePath)
	assert.NotNil(t, repo.data)
	assert.Len(t, repo.data, 3) // 3 date keys in sample file
}

func TestNewJSONFileRepository_FileNotFound(t *testing.T) {
	nonExistentFile := filepath.Join("testdata", "nonexistent.json")

	repo, err := NewJSONFileRepository(nonExistentFile)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataSourceNotFound)
}

func TestNewJSONFileRepository_EmptyFilePath(t *testing.T) {
	repo, err := NewJSONFileRepository("")

	assert.Nil(t, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file path cannot be empty")
}

func TestNewJSONFileRepository_CorruptedJSON(t *testing.T) {
	corruptedFile := filepath.Join("testdata", "posts_corrupted.json")

	repo, err := NewJSONFileRepository(corruptedFile)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataCorrupted)
}

func TestNewJSONFileRepository_InvalidData(t *testing.T) {
	invalidFile := filepath.Join("testdata", "posts_invalid_data.json")

	repo, err := NewJSONFileRepository(invalidFile)

	assert.Nil(t, repo)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDataCorrupted)
}

func TestGetPostForDate_ValidDate(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		dateKey string
	}{
		{
			name:    "date with multiple posts",
			dateKey: "09-14",
		},
		{
			name:    "date with single post",
			dateKey: "02-14",
		},
		{
			name:    "date with three posts",
			dateKey: "12-31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post, err := repo.GetPostForDate(ctx, tt.dateKey)

			require.NoError(t, err)
			assert.NotNil(t, post)

			// Verify post has correct anniversary date key
			assert.Equal(t, tt.dateKey, post.AnniversaryDateKey())
			assert.NotEmpty(t, post.URI())
			assert.False(t, post.Date().IsZero())
		})
	}
}

func TestGetPostForDate_InvalidDate(t *testing.T) {
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
			post, err := repo.GetPostForDate(ctx, tt.dateKey)

			assert.Nil(t, post)
			require.Error(t, err)

			var repoErr ports.RepositoryError
			assert.ErrorAs(t, err, &repoErr)
			assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrInvalidDateFormat)
		})
	}
}

func TestGetPostForDate_DateNotFound(t *testing.T) {
	repo := createTestRepository(t)
	ctx := context.Background()

	post, err := repo.GetPostForDate(ctx, "01-01")

	assert.Nil(t, post)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), ports.ErrDateNotFound)
}

func TestGetPostForDate_ContextCanceled(t *testing.T) {
	repo := createTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	post, err := repo.GetPostForDate(ctx, "09-14")

	assert.Nil(t, post)
	require.Error(t, err)

	var repoErr ports.RepositoryError
	assert.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, repoErr.Unwrap(), context.Canceled)
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
}

func TestValidateDataSource_FileNotFound(t *testing.T) {
	repo := &JSONFileRepository{
		filePath: filepath.Join("testdata", "nonexistent.json"),
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
	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo := &JSONFileRepository{
		filePath: sampleFile,
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
	sampleFile := filepath.Join("testdata", "posts_sample.json")

	repo, err := NewJSONFileRepository(sampleFile)
	require.NoError(t, err)
	return repo
}

// Benchmark tests

func BenchmarkGetPostForDate(b *testing.B) {
	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo, err := NewJSONFileRepository(sampleFile)
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetPostForDate(ctx, "09-14")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListAvailableDates(b *testing.B) {
	sampleFile := filepath.Join("testdata", "posts_sample.json")
	repo, err := NewJSONFileRepository(sampleFile)
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
