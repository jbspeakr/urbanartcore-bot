package repository

import (
	"testing"

	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// TestJSONFileRepository_ImplementsInterfaces verifies that JSONFileRepository
// correctly implements all required interfaces.
func TestJSONFileRepository_ImplementsInterfaces(t *testing.T) {
	repo := &JSONFileRepository{
		filePath: "testdata/posts_sample.json",
		data:     make(postsData),
	}

	// Test that JSONFileRepository implements PostRepository
	var _ ports.PostRepository = repo

	// Test that JSONFileRepository implements StatsProvider
	var _ ports.StatsProvider = repo

	t.Log("JSONFileRepository correctly implements PostRepository and StatsProvider interfaces")
}
