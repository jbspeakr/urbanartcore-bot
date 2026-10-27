package site

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

// fakeSite serves the fixture catalogue and the Banksy post, with the
// fixture's permalinks rewritten to point at the test server.
type fakeSite struct {
	*httptest.Server
	catalogueHits atomic.Int32
	userAgent     atomic.Value
}

func newFakeSite(t *testing.T) *fakeSite {
	t.Helper()
	index := readFixture(t, "index.json")
	post := readFixture(t, "banksy-revealed.json")

	s := &fakeSite{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.userAgent.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/index.json":
			s.catalogueHits.Add(1)
			_, _ = w.Write([]byte(strings.ReplaceAll(string(index), "https://www.urbanartcore.com", s.URL)))
		case "/banksy-revealed/index.json":
			_, _ = w.Write(post)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestGetPostForDate_LiveCatalogue(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL+"/", WithRandom(func(n int) int { return n - 1 }))
	require.NoError(t, err)
	ctx := context.Background()

	post, err := client.GetPostForDate(ctx, "09-14")
	require.NoError(t, err)
	assert.Equal(t, SourceLive, client.Source())
	assert.Equal(t, srv.URL+"/biggest-marker-worldwide/", post.URI(), "the random pick chooses among the day's posts")
	assert.Equal(t, "Graffiti", post.Article().Section)

	post, err = client.GetPostForDate(ctx, "08-28")
	require.NoError(t, err)
	article := post.Article()
	assert.Equal(t, "ETAM Crew Exhibition in Vienna", article.Title)
	assert.Equal(t, "After releasing their Urban Art Guide Vienna, Inoperable Gallery shows ETAM Crew.", article.Description)
	assert.Equal(t, "Street Art", article.Section)
	assert.Equal(t, []string{"Sainer", "ETAM"}, article.Tags.Artists)
	assert.Equal(t, []string{"Europe"}, article.Tags.Places)
	assert.Equal(t, []string{"Exhibition"}, article.Tags.Formats)

	count, err := client.GetPostCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "the entry with a broken date is skipped")

	dates, err := client.ListAvailableDates(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"08-28", "09-14"}, dates)

	assert.EqualValues(t, 1, srv.catalogueHits.Load(), "the catalogue is fetched once")
	assert.Contains(t, srv.userAgent.Load(), "github.com/jbspeakr/urbanartcore-bot")
}

func TestGetPostForDate_RestDayAndBadKey(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	_, err = client.GetPostForDate(context.Background(), "12-25")
	assert.ErrorIs(t, err, ports.ErrDateNotFound)

	_, err = client.GetPostForDate(context.Background(), "2011-09-14")
	assert.ErrorIs(t, err, ports.ErrInvalidDateFormat)
}

func TestNewClient_MakesNoRequest(t *testing.T) {
	srv := newFakeSite(t)
	_, err := NewClient(srv.URL)
	require.NoError(t, err)
	assert.EqualValues(t, 0, srv.catalogueHits.Load())
}

func TestNewClient_RejectsBadBaseURL(t *testing.T) {
	_, err := NewClient("www.urbanartcore.com")
	assert.Error(t, err)

	client, err := NewClient("  ")
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, client.baseURL)
}

func TestLoad_FallsBackToSnapshot(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"site down":     func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) },
		"format drift":  func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"posts":[]}`)) },
		"empty catalog": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()

			client, err := NewClient(srv.URL, WithSnapshot(readFixture(t, "index.json")))
			require.NoError(t, err)

			post, err := client.GetPostForDate(context.Background(), "08-28")
			require.NoError(t, err)
			assert.Equal(t, SourceSnapshot, client.Source())
			assert.Equal(t, "https://www.urbanartcore.com/etam-crew-exhibition-in-vienna/", post.URI())
		})
	}
}

func TestLoad_FailsWithoutAnyCatalogue(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	client, err := NewClient(srv.URL, WithSnapshot([]byte(`not json`)))
	require.NoError(t, err)

	err = client.ValidateDataSource(context.Background())
	assert.ErrorIs(t, err, ports.ErrDataSourceNotFound)
	_, err = client.GetPostForDate(context.Background(), "08-28")
	assert.Error(t, err, "the failure is remembered")
}

// TestBuiltInSnapshot checks the snapshot shipped in the binary: it parses,
// holds the archive, and leaves the known rest days empty.
func TestBuiltInSnapshot(t *testing.T) {
	var entries []indexEntry
	require.NoError(t, json.Unmarshal(snapshot, &entries))

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	ctx := context.Background()

	count, err := client.GetPostCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, SourceSnapshot, client.Source())
	assert.Greater(t, count, 900)
	assert.Equal(t, len(entries), count, "every snapshot entry is a valid post")

	post, err := client.GetPostForDate(ctx, "09-14")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(post.URI(), "https://www.urbanartcore.com/"))
}

func TestReadPost_FullArticle(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL, WithRandom(func(int) int { return 0 }))
	require.NoError(t, err)
	ctx := context.Background()

	catalogued, err := client.GetPostForDate(ctx, "09-14")
	require.NoError(t, err)
	require.Equal(t, srv.URL+"/banksy-revealed/", catalogued.URI())

	full, err := client.ReadPost(ctx, catalogued)
	require.NoError(t, err)
	assert.Equal(t, catalogued.URI(), full.URI())
	assert.Equal(t, catalogued.Date(), full.Date())

	article := full.Article()
	assert.Equal(t, "Banksy Revealed!", article.Title)
	assert.Equal(t, "An original, eight years old Banksy mural was recently revealed by Berlin-based artist Brad Downey and curator Andrian Nabi!", article.Description)
	assert.Equal(t, "Street Art", article.Section)
	assert.Equal(t, domain.Tags{
		Artists:    []string{"Banksy", "Brad Downey"},
		Places:     []string{"Berlin"},
		Techniques: []string{"Mural Art"},
		Formats:    []string{"Exhibition"},
		Topics:     []string{"Art Restoration"},
	}, article.Tags)
	assert.Equal(t, domain.Cover{
		URL:    "https://www.urbanartcore.com/banksy-revealed/banksy-revealation1.jpg",
		Alt:    "Banksy Revealed!",
		Width:  1200,
		Height: 720,
	}, article.Cover)
	assert.Empty(t, article.Credit)
	assert.Equal(t,
		"After eight years and several new layers of paint, Berlin-based artist Brad Downey and curator Andrian Nabi recently revealed an original Banksy mural!\n\n"+
			"The wall was painted during the Bach Jumps festival in 2003 & then covered.\n\n"+
			"More\n\n"+
			"Photos by the artists",
		article.Body)

	assert.Empty(t, catalogued.Article().Body, "the catalogue entry is not changed")
}

func TestReadPost_Errors(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	missing, err := domain.NewPost(srv.URL+"/no-such-post/", "2010-01-01")
	require.NoError(t, err)
	_, err = client.ReadPost(context.Background(), missing)
	assert.ErrorContains(t, err, "404")

	_, err = client.ReadPost(context.Background(), nil)
	assert.Error(t, err)
}

func TestReadPost_EmptyArticleIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"title":"","markdown":""}`))
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	post, err := domain.NewPost(srv.URL+"/empty/", "2010-01-01")
	require.NoError(t, err)
	_, err = client.ReadPost(context.Background(), post)
	assert.Error(t, err)
}

func TestReadPost_CancelledContext(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	post, err := domain.NewPost(srv.URL+"/banksy-revealed/", "2011-09-14")
	require.NoError(t, err)
	_, err = client.ReadPost(ctx, post)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestPlainText(t *testing.T) {
	assert.Equal(t, "", PlainText(""))
	assert.Equal(t, "One. Two.\n\nThree", PlainText("One.\nTwo.\n\n\n> Three"))
	assert.Equal(t, "Link text and more", PlainText("[Link text](<https://example.org/a b>) and **more**"))
}

func TestGetPostsForDate(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	posts, err := client.GetPostsForDate(context.Background(), "09-14")
	require.NoError(t, err)
	require.Len(t, posts, 2)
	posts[0] = nil
	again, err := client.GetPostsForDate(context.Background(), "09-14")
	require.NoError(t, err)
	assert.NotNil(t, again[0], "callers get a copy")

	_, err = client.GetPostsForDate(context.Background(), "12-25")
	assert.ErrorIs(t, err, ports.ErrDateNotFound)
	_, err = client.GetPostsForDate(context.Background(), "9-14")
	assert.ErrorIs(t, err, ports.ErrInvalidDateFormat)
}
