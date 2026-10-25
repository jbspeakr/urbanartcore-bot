package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// TestMastodonAdapter_Publish_SendsLanguageAndIdempotencyKey checks the status
// carries a language and that two publishes of the same post on the same day
// send the same Idempotency-Key.
func TestMastodonAdapter_Publish_SendsLanguageAndIdempotencyKey(t *testing.T) {
	var keys []string
	var bodies []map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/statuses" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		bodies = append(bodies, body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","url":"https://mastodon.social/@uac/1"}`))
	}))
	defer server.Close()

	adapter := &MastodonAdapter{server: server.URL, accessToken: testAccessToken, httpClient: &http.Client{}}

	for _, text := range []string{"First draft https://www.example.com/test", "Second draft https://www.example.com/test"} {
		content, err := domain.NewGeneratedContent(text, domain.PlatformMastodon, []string{"StreetArt", "Graffiti"}, "", "https://www.example.com/test")
		require.NoError(t, err)
		_, err = adapter.Publish(context.Background(), content)
		require.NoError(t, err)
	}

	require.Len(t, keys, 2)
	assert.NotEmpty(t, keys[0])
	assert.Equal(t, keys[0], keys[1])
	assert.Equal(t, "en", bodies[0]["language"])
	assert.Equal(t, "public", bodies[0]["visibility"])
	assert.Equal(t, "First draft https://www.example.com/test\n\n#StreetArt #Graffiti", bodies[0]["status"])
}

// TestMastodonAdapter_ValidateContent_URLCountsAs23 checks a long URL does not
// push a status over the limit.
func TestMastodonAdapter_ValidateContent_URLCountsAs23(t *testing.T) {
	adapter := &MastodonAdapter{server: testServer, accessToken: testAccessToken, httpClient: &http.Client{}}
	url := "https://www.urbanartcore.com/" + strings.Repeat("x", 80) + "/"
	content, err := domain.NewGeneratedContent(strings.Repeat("A", 460)+" "+url, domain.PlatformMastodon, nil, "", url)
	require.NoError(t, err)

	assert.NoError(t, adapter.ValidateContent(context.Background(), content))
}

// TestBlueskyAdapter_ValidateContent_GraphemeLimit checks Bluesky accepts 300
// multi-byte graphemes, which the old byte count rejected.
func TestBlueskyAdapter_ValidateContent_GraphemeLimit(t *testing.T) {
	adapter := &BlueskyAdapter{}
	text := strings.Repeat("ü", 150) + strings.Repeat("🎨", 150)
	content, err := domain.NewGeneratedContent(text, domain.PlatformBluesky, nil, "", "https://www.example.com/test")
	require.NoError(t, err)

	assert.NoError(t, adapter.ValidateContent(context.Background(), content))
	assert.Equal(t, 300, adapter.GetCharacterLimit())
}

// TestBlueskyAdapter_Publish_SendsLangs checks the post record carries langs
// and the rendered text with hashtags and without the URL.
func TestBlueskyAdapter_Publish_SendsLangs(t *testing.T) {
	var record map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessJwt": "a", "refreshJwt": "r", "handle": "test.bsky.social", "did": "did:plc:test123",
			})
		case "/xrpc/com.atproto.repo.createRecord":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			record, _ = req["record"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri": "at://did:plc:test123/app.bsky.feed.post/abc123", "cid": "bafy",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	adapter := &BlueskyAdapter{handle: "test.bsky.social", password: "pw", pdsHost: server.URL, httpClient: &http.Client{}}
	// The link card fetch goes to the mock server and fails, which Publish tolerates
	content, err := domain.NewGeneratedContent("In 2011 a wall in Berlin https://example.invalid/post", domain.PlatformBluesky, []string{"StreetArt"}, "", server.URL+"/post")
	require.NoError(t, err)

	_, err = adapter.Publish(context.Background(), content)
	require.NoError(t, err)

	require.NotNil(t, record)
	assert.Equal(t, []any{"en"}, record["langs"])
	assert.Equal(t, "In 2011 a wall in Berlin #StreetArt", record["text"])
}
