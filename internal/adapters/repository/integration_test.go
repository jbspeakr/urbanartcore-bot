//go:build integration
// +build integration

package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeberg.org/brennenstuhl/uac-socializer/internal/infrastructure"
)

// TestJSONFileRepository_RealData tests the repository with the actual posts_by_date.json file.
// Run with: go test -tags=integration ./internal/adapters/repository/
func TestJSONFileRepository_RealData(t *testing.T) {
	realDataFile := filepath.Join("..", "..", "..", "data", "posts_by_date.json")
	logger := createTestLogger(t)

	// Test repository creation with real data
	repo, err := NewJSONFileRepository(realDataFile, logger)
	require.NoError(t, err, "Should be able to create repository with real data")
	assert.NotNil(t, repo)

	ctx := context.Background()

	// Test data source validation
	err = repo.ValidateDataSource(ctx)
	require.NoError(t, err, "Real data should be valid")

	// Test getting posts for a known date
	posts, err := repo.GetPostsByDate(ctx, "09-14")
	require.NoError(t, err, "Should be able to get posts for 09-14")

	// We know from the sample data that 09-14 should have posts
	if len(posts) > 0 {
		t.Logf("Found %d posts for 09-14", len(posts))

		// Verify post structure
		for i, post := range posts {
			assert.NotEmpty(t, post.URI(), "Post %d should have URI", i)
			assert.False(t, post.Date().IsZero(), "Post %d should have valid date", i)
			assert.Equal(t, "09-14", post.AnniversaryDateKey(), "Post %d should have correct anniversary date", i)
		}
	} else {
		t.Log("No posts found for 09-14 in real data")
	}

	// Test listing available dates
	dates, err := repo.ListAvailableDates(ctx)
	require.NoError(t, err, "Should be able to list available dates")
	assert.Greater(t, len(dates), 0, "Should have some dates available")
	t.Logf("Total dates available: %d", len(dates))

	// Test getting total post count
	totalPosts, err := repo.GetPostCount(ctx)
	require.NoError(t, err, "Should be able to get post count")
	assert.Greater(t, totalPosts, 0, "Should have some posts")
	t.Logf("Total posts: %d", totalPosts)
}

// BenchmarkRealData_GetPostsByDate benchmarks retrieval with real data.
// Run with: go test -tags=integration -bench=BenchmarkRealData ./internal/adapters/repository/
func BenchmarkRealData_GetPostsByDate(b *testing.B) {
	realDataFile := filepath.Join("..", "..", "..", "data", "posts_by_date.json")
	logger, err := infrastructure.NewStructuredLogger(&infrastructure.LoggerConfig{
		Level:  infrastructure.LevelError, // Minimal logging for benchmarks
		Format: "json",
		Output: "stderr",
	})
	require.NoError(b, err)

	repo, err := NewJSONFileRepository(realDataFile, logger)
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
