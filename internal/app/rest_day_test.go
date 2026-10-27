package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/adapters/site"
	"github.com/jbspeakr/urbanartcore-bot/internal/app"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// TestRestDays_Snapshot runs the publish service against the built-in
// catalogue snapshot on calendar days that have no anniversary post. Each
// must end as a rest day without touching the generator or any publisher.
func TestRestDays_Snapshot(t *testing.T) {
	// The site is down, so the client falls back to the snapshot
	down := httptest.NewServer(http.NotFoundHandler())
	defer down.Close()
	repo, err := site.NewClient(down.URL)
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
