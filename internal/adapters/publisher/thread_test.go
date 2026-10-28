package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

func TestBlueskyPublish_ReplyInThread(t *testing.T) {
	var record map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_ = json.NewEncoder(w).Encode(map[string]any{"accessJwt": "a", "refreshJwt": "r", "handle": "uac.bsky.social", "did": "did:plc:uac"})
		case "/xrpc/com.atproto.repo.createRecord":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			record = req["record"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{"uri": "at://did:plc:uac/app.bsky.feed.post/reply1", "cid": "bafyreply"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter := &BlueskyAdapter{handle: "uac.bsky.social", password: "p", pdsHost: server.URL, httpClient: &http.Client{}}

	content := createValidContent(t).InReplyTo(
		"at://did:plc:uac/app.bsky.feed.post/root bafyroot",
		"at://did:plc:uac/app.bsky.feed.post/prev bafyprev")
	result, err := adapter.Publish(context.Background(), content)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"root":   map[string]any{"uri": "at://did:plc:uac/app.bsky.feed.post/root", "cid": "bafyroot"},
		"parent": map[string]any{"uri": "at://did:plc:uac/app.bsky.feed.post/prev", "cid": "bafyprev"},
	}, record["reply"])
	assert.Equal(t, "at://did:plc:uac/app.bsky.feed.post/reply1 bafyreply", result.RemoteRef(), "the next reply can answer this one")
}

func TestBlueskyReply(t *testing.T) {
	reply, err := blueskyReply("", "at://x/app.bsky.feed.post/1 cid1")
	require.NoError(t, err)
	assert.Equal(t, reply["root"], reply["parent"], "a first reply answers the root")

	_, err = blueskyReply("at://x/1 cid", "https://bsky.app/x cid")
	assert.Error(t, err)
	_, err = blueskyReply("", "at://x/1")
	assert.Error(t, err)
}

func TestMastodonPublish_ReplyInThread(t *testing.T) {
	f := newFakeMastodonMedia(t)
	content, err := domain.NewGeneratedContent("Second part.", domain.PlatformMastodon, nil, "", "https://www.urbanartcore.com/x/")
	require.NoError(t, err)

	result, err := f.adapter().Publish(context.Background(), content.InReplyTo("100", "101"))
	require.NoError(t, err)
	assert.Equal(t, "101", f.status["in_reply_to_id"])
	assert.Equal(t, "s1", result.RemoteRef())
}
