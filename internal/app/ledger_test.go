package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// memLedger is an in-memory ports.Ledger.
type memLedger struct {
	records   []domain.PublishRecord
	readErr   error
	appendErr error
	since     time.Time
}

func (l *memLedger) Records(ctx context.Context, since time.Time) ([]domain.PublishRecord, error) {
	l.since = since
	if l.readErr != nil {
		return nil, l.readErr
	}
	var out []domain.PublishRecord
	for _, r := range l.records {
		if r.Date >= since.Format(domain.DateLayout) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (l *memLedger) Append(ctx context.Context, record domain.PublishRecord) error {
	if l.appendErr != nil {
		return l.appendErr
	}
	l.records = append(l.records, record)
	return nil
}

// okPublisher accepts every post and counts them.
type okPublisher struct {
	ports.SocialPublisher
	platform  string
	published []string
}

func (p *okPublisher) Platform() string { return p.platform }

func (p *okPublisher) ValidateContent(ctx context.Context, content *domain.GeneratedContent) error {
	return nil
}

func (p *okPublisher) Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error) {
	p.published = append(p.published, content.URI())
	result, err := domain.NewPlatformResult(p.platform, domain.StatusSuccess, content)
	if err != nil {
		return nil, err
	}
	result.SetSuccess("https://"+p.platform+".example/status/"+content.URI(), time.Millisecond)
	return result, nil
}

var (
	ledgerNow = time.Date(2026, 9, 14, 11, 55, 0, 0, time.UTC)
	thalys    = createTestPost("https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/", "2009-09-14")
	marker    = createTestPost("https://www.urbanartcore.com/biggest-marker-worldwide/", "2010-09-14")
	revealed  = createTestPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14")
)

type ledgerRig struct {
	service    *PublishService
	ledger     *memLedger
	publishers map[string]*okPublisher
}

func newLedgerRig(t *testing.T, ledger *memLedger, posts ...*domain.Post) *ledgerRig {
	t.Helper()
	repo := &MockPostRepository{}
	repo.On("GetPostsForDate", context.Background(), "09-14").Return(posts, nil)

	rig := &ledgerRig{ledger: ledger, publishers: map[string]*okPublisher{}}
	publishers := map[string]ports.SocialPublisher{}
	for _, platform := range []string{domain.PlatformBluesky, domain.PlatformMastodon} {
		pub := &okPublisher{platform: platform}
		rig.publishers[platform] = pub
		publishers[platform] = pub
	}
	gen := &sequenceGenerator{drafts: []string{"A story from Paris."}}
	rig.service = NewPublishService(repo, gen, publishers).WithLedger(ledger)
	rig.service.now = func() time.Time { return ledgerNow }
	rig.service.intn = func(n int) int { return 0 }
	return rig
}

func (r *ledgerRig) publish(t *testing.T, dryRun bool) *PublishResponse {
	t.Helper()
	resp, err := r.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}, DryRun: dryRun})
	require.NoError(t, err)
	return resp
}

func TestLedger_RecordsEveryPublishedPost(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, thalys, marker)

	resp := rig.publish(t, false)
	require.NoError(t, resp.Error)
	require.Len(t, rig.ledger.records, 2)
	byPlatform := map[string]domain.PublishRecord{}
	for _, r := range rig.ledger.records {
		byPlatform[r.Platform] = r
	}
	bluesky := byPlatform[domain.PlatformBluesky]
	assert.Equal(t, "2026-09-14/on-this-day/graffiti-meets-thalys-in-paris/bluesky", bluesky.Key)
	assert.Equal(t, "2026-09-14", bluesky.Date)
	assert.Equal(t, domain.ProgramOnThisDay, bluesky.Program)
	assert.Equal(t, thalys.URI(), bluesky.PostURI)
	assert.Equal(t, "https://bluesky.example/status/"+thalys.URI(), bluesky.RemoteURL)
	assert.Contains(t, bluesky.Text, "A story from Paris.")
	assert.False(t, bluesky.PublishedAt.IsZero())
	assert.Equal(t, "2025-09-14", rig.ledger.since.Format(domain.DateLayout), "history covers the repeat window")
}

func TestLedger_RerunPostsNothing(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, thalys, marker)
	rig.publish(t, false)

	resp := rig.publish(t, false)
	assert.True(t, resp.Published)
	assert.Len(t, rig.publishers[domain.PlatformBluesky].published, 1)
	assert.Len(t, rig.publishers[domain.PlatformMastodon].published, 1)
	assert.Len(t, rig.ledger.records, 2)
}

func TestLedger_RerunCompletesAMissingPlatform(t *testing.T) {
	ledger := &memLedger{records: []domain.PublishRecord{{
		Date: "2026-09-14", Platform: domain.PlatformBluesky, PostURI: thalys.URI(), RemoteURL: "https://bsky.app/x",
	}}}
	rig := newLedgerRig(t, ledger, marker, thalys)

	resp := rig.publish(t, false)
	assert.False(t, resp.Published)
	assert.Empty(t, rig.publishers[domain.PlatformBluesky].published, "Bluesky already has today's post")
	assert.Equal(t, []string{thalys.URI()}, rig.publishers[domain.PlatformMastodon].published,
		"Mastodon gets the post Bluesky got earlier today")
}

func TestLedger_PassesOverRecentlyFeaturedPosts(t *testing.T) {
	ledger := &memLedger{records: []domain.PublishRecord{
		{Date: "2025-09-14", Platform: domain.PlatformBluesky, PostURI: thalys.URI()},
		{Date: "2025-12-01", Platform: domain.PlatformMastodon, PostURI: marker.URI()},
	}}
	rig := newLedgerRig(t, ledger, thalys, marker, revealed)

	rig.publish(t, false)
	assert.Equal(t, []string{revealed.URI()}, rig.publishers[domain.PlatformBluesky].published)
}

func TestLedger_RepeatsWhenEveryPostIsRecent(t *testing.T) {
	ledger := &memLedger{records: []domain.PublishRecord{
		{Date: "2025-09-14", Platform: domain.PlatformBluesky, PostURI: thalys.URI()},
	}}
	rig := newLedgerRig(t, ledger, thalys)

	rig.publish(t, false)
	assert.Equal(t, []string{thalys.URI()}, rig.publishers[domain.PlatformBluesky].published)
}

func TestLedger_DryRunWritesNothing(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, thalys)

	resp := rig.publish(t, true)
	assert.NotEmpty(t, resp.Generated)
	assert.Empty(t, rig.ledger.records)
}

func TestLedger_UnreadableLedgerStopsTheRun(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{readErr: errors.New("403 AccessDenied")}, thalys)

	_, err := rig.service.PublishAnniversaryPosts(context.Background(), &PublishRequest{DateKey: "09-14", Platforms: []string{"all"}})
	assert.ErrorContains(t, err, "failed to read the ledger")
	assert.Empty(t, rig.publishers[domain.PlatformBluesky].published)
}

func TestLedger_FailedWriteDoesNotFailThePublish(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{appendErr: errors.New("503")}, thalys)

	resp := rig.publish(t, false)
	assert.NoError(t, resp.Error)
	assert.Len(t, rig.publishers[domain.PlatformBluesky].published, 1)
}

func TestSlotDate(t *testing.T) {
	ps := &PublishService{now: func() time.Time { return time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC) }}
	assert.Equal(t, "2026-09-14", ps.slotDate("09-14").Format(domain.DateLayout))
	assert.Equal(t, "2026-03-01", ps.slotDate("02-29").Format(domain.DateLayout), "February 29 outside a leap year")
}
