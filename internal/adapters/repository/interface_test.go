package repository

import (
	"log/slog"
	"os"
	"testing"

	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// TestJSONFileRepository_ImplementsInterfaces verifies that JSONFileRepository
// correctly implements all required interfaces.
func TestJSONFileRepository_ImplementsInterfaces(t *testing.T) {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	repo := &JSONFileRepository{
		filePath: "testdata/posts_sample.json",
		logger:   logger,
		data:     make(postsData),
	}

	// Test that JSONFileRepository implements PostRepository
	var _ ports.PostRepository = repo

	// Test that JSONFileRepository implements StatsProvider
	var _ ports.StatsProvider = repo

	t.Log("JSONFileRepository correctly implements PostRepository and StatsProvider interfaces")
}
