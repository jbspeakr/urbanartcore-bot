package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// fakeMastodonMedia serves a cover image and the media and status
// endpoints, recording what the adapter sends.
type fakeMastodonMedia struct {
	*httptest.Server
	mu          sync.Mutex
	uploadCode  int // status of POST /api/v2/media
	pollsLeft   int // GET /api/v1/media/:id answers 206 this often before 200
	description string
	fileType    string
	fileBytes   int
	status      map[string]any
}

func newFakeMastodonMedia(t *testing.T) *fakeMastodonMedia {
	t.Helper()
	f := &fakeMastodonMedia{uploadCode: http.StatusOK}
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 512)...)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/covers/wall.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(jpeg)
		case "/api/v2/media":
			assert.Equal(t, "Bearer "+testAccessToken, r.Header.Get("Authorization"))
			require.NoError(t, r.ParseMultipartForm(1<<20))
			f.description = r.FormValue("description")
			file, header, err := r.FormFile("file")
			require.NoError(t, err)
			data, _ := io.ReadAll(file)
			f.fileBytes = len(data)
			f.fileType = header.Header.Get("Content-Type")
			w.WriteHeader(f.uploadCode)
			switch f.uploadCode {
			case http.StatusAccepted:
				_, _ = io.WriteString(w, `{"id":"m1","url":null}`)
			case http.StatusOK:
				_, _ = io.WriteString(w, `{"id":"m1","url":"https://files.example/m1.jpg"}`)
			}
		case "/api/v1/media/m1":
			if f.pollsLeft > 0 {
				f.pollsLeft--
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, `{"id":"m1","url":null}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"m1","url":"https://files.example/m1.jpg"}`)
		case "/api/v1/statuses":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&f.status))
			_, _ = io.WriteString(w, `{"id":"s1","url":"https://mastodon.example/@uac/s1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeMastodonMedia) adapter() *MastodonAdapter {
	return &MastodonAdapter{server: f.URL, accessToken: testAccessToken, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func (f *fakeMastodonMedia) content(t *testing.T) *domain.GeneratedContent {
	t.Helper()
	content, err := domain.NewGeneratedContent("Brad Downey uncovered a 2003 Banksy in Berlin. https://www.urbanartcore.com/banksy-revealed/",
		domain.PlatformMastodon, []string{"#Banksy"}, "prompt", "https://www.urbanartcore.com/banksy-revealed/")
	require.NoError(t, err)
	return content.WithImages([]domain.Image{{URL: f.URL + "/covers/wall.jpg", Alt: "A rat stencil on a brick wall. Photo: Brad Downey."}})
}

func shortMediaPolls(t *testing.T) {
	t.Helper()
	old := mediaPollInterval
	mediaPollInterval = time.Millisecond
	t.Cleanup(func() { mediaPollInterval = old })
}

func TestMastodonPublish_AttachesImageWithAltText(t *testing.T) {
	f := newFakeMastodonMedia(t)

	result, err := f.adapter().Publish(context.Background(), f.content(t))
	require.NoError(t, err)
	assert.True(t, result.IsSuccess())

	assert.Equal(t, "A rat stencil on a brick wall. Photo: Brad Downey.", f.description)
	assert.Equal(t, "image/jpeg", f.fileType)
	assert.Equal(t, 516, f.fileBytes)
	assert.Equal(t, []any{"m1"}, f.status["media_ids"])
	assert.Equal(t, "en", f.status["language"])
}

func TestMastodonPublish_WaitsForProcessing(t *testing.T) {
	shortMediaPolls(t)
	f := newFakeMastodonMedia(t)
	f.uploadCode = http.StatusAccepted
	f.pollsLeft = 2

	_, err := f.adapter().Publish(context.Background(), f.content(t))
	require.NoError(t, err)
	assert.Equal(t, []any{"m1"}, f.status["media_ids"])
	assert.Zero(t, f.pollsLeft, "polled until the upload was processed")
}

func TestMastodonPublish_PostsWithoutImageWhenUploadFails(t *testing.T) {
	shortMediaPolls(t)
	for name, setup := range map[string]func(f *fakeMastodonMedia){
		"upload rejected":    func(f *fakeMastodonMedia) { f.uploadCode = http.StatusUnprocessableEntity },
		"never processed":    func(f *fakeMastodonMedia) { f.uploadCode = http.StatusAccepted; f.pollsLeft = 100 },
		"image not found":    func(f *fakeMastodonMedia) {},
		"image not an image": func(f *fakeMastodonMedia) {},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeMastodonMedia(t)
			setup(f)
			content := f.content(t)
			switch name {
			case "image not found":
				content.WithImages([]domain.Image{{URL: f.URL + "/covers/missing.jpg", Alt: "x"}})
			case "image not an image":
				content.WithImages([]domain.Image{{URL: f.URL + "/api/v1/media/m1", Alt: "x"}})
			}

			result, err := f.adapter().Publish(context.Background(), content)
			require.NoError(t, err)
			assert.True(t, result.IsSuccess())
			require.NotNil(t, f.status)
			assert.NotContains(t, f.status, "media_ids")
			assert.True(t, strings.HasPrefix(f.status["status"].(string), "Brad Downey"))
		})
	}
}

func TestImageFilename(t *testing.T) {
	assert.Equal(t, "wall.jpg", imageFilename("https://www.urbanartcore.com/a/wall.jpg?w=1200"))
	assert.Equal(t, "image", imageFilename(""))
}
