package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/repository"
	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// TestRestDays_RealData runs the publish service against the shipped data
// file on the calendar days that have no anniversary post. Each must end as
// a rest day without touching the generator or any publisher.
func TestRestDays_RealData(t *testing.T) {
	repo, err := repository.NewJSONFileRepository(filepath.Join("..", "..", "data", "posts_by_date.json"))
	require.NoError(t, err)

	// nil generator and publishers: any call into them would panic
	service := app.NewPublishService(repo, nilGenerator{}, map[string]ports.SocialPublisher{})

	for _, dateKey := range []string{"01-01", "05-01", "12-24", "12-25"} {
		t.Run(dateKey, func(t *testing.T) {
			resp, err := service.PublishAnniversaryPosts(context.Background(), &app.PublishRequest{
				DateKey:   dateKey,
				Platforms: []string{"all"},
				DryRun:    true,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.True(t, resp.RestDay)
		})
	}
}

// nilGenerator satisfies ports.ContentGenerator and panics if any method is called.
type nilGenerator struct{ ports.ContentGenerator }
