package generator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// criticServer answers chat requests with the given contents in turn,
// repeating the last, and records the requests.
func criticServer(t *testing.T, contents ...string) (*httptest.Server, *[]chatRequest) {
	t.Helper()
	var requests []chatRequest
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests = append(requests, req)
		i := min(int(calls.Add(1))-1, len(contents)-1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": contents[i]}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func banksyPost(t *testing.T) *domain.Post {
	t.Helper()
	post, err := domain.NewPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14")
	require.NoError(t, err)
	return post.WithArticle(domain.Article{
		Title: "Banksy Revealed!",
		Body:  "Brad Downey and curator Andrian Nabi revealed an original Banksy mural in Berlin, painted in 2003.",
		Tags:  domain.Tags{Artists: []string{"Banksy", "Brad Downey"}, Places: []string{"Berlin"}},
	})
}

func banksyDraft(t *testing.T) *domain.GeneratedContent {
	t.Helper()
	content, err := domain.NewGeneratedContent("Brad Downey uncovered a 2003 Banksy in Berlin. The wall is red.", domain.PlatformBluesky,
		[]string{"#Banksy"}, "prompt", "https://www.urbanartcore.com/banksy-revealed/")
	require.NoError(t, err)
	return content.WithClaims([]string{"Brad Downey uncovered a Banksy mural.", "The wall is red."})
}

func TestMistralCritic_Review(t *testing.T) {
	srv, requests := criticServer(t, `{
		"verdicts": [
			{"claim": "Brad Downey uncovered a Banksy mural.", "supported": true, "evidence": "Brad Downey and curator Andrian Nabi revealed an original Banksy mural"},
			{"claim": "The wall is red.", "supported": false, "evidence": "The story names no colour."}
		],
		"voice_score": 3,
		"voice_problems": ""
	}`)
	critic, err := NewMistralCritic("test-key", "ministral-8b-2512", srv.URL, 5*time.Second)
	require.NoError(t, err)

	review, err := critic.Review(context.Background(), banksyPost(t), banksyDraft(t))
	require.NoError(t, err)
	assert.False(t, review.Approved())
	assert.Equal(t, domain.VoiceScoreGood, review.VoiceScore)
	require.Len(t, review.Unsupported(), 1)
	assert.Equal(t, "The wall is red.", review.Unsupported()[0].Claim)

	require.Len(t, *requests, 1)
	req := (*requests)[0]
	assert.Equal(t, "ministral-8b-2512", req.Model)
	assert.Zero(t, req.Temperature)
	require.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "draft_review", req.ResponseFormat.JSONSchema.Name)
	require.Len(t, req.Messages, 2)
	assert.Contains(t, req.Messages[0].Content, "supported only when THE STORY states it")
	prompt := req.Messages[1].Content
	assert.Contains(t, prompt, "THE STORY:\nTitle: Banksy Revealed!\nArtists: Banksy, Brad Downey\nPlaces: Berlin\nText:\nBrad Downey and curator")
	assert.Contains(t, prompt, "Published: September 14, 2011\nLink: https://www.urbanartcore.com/banksy-revealed/")
	assert.Contains(t, prompt, "THE DRAFT:\nBrad Downey uncovered a 2003 Banksy in Berlin. The wall is red.\n")
	assert.NotContains(t, prompt, "#Banksy", "the critic reads the text without hashtags")
	assert.Contains(t, prompt, "CLAIMS:\n- Brad Downey uncovered a Banksy mural.\n- The wall is red.\n")
}

func TestMistralCritic_RetriesUnusableVerdict(t *testing.T) {
	srv, requests := criticServer(t, `not json`, `{"verdicts": [], "voice_score": 2, "voice_problems": ""}`)
	critic, err := NewMistralCritic("test-key", "ministral-8b-2512", srv.URL, 5*time.Second)
	require.NoError(t, err)

	review, err := critic.Review(context.Background(), banksyPost(t), banksyDraft(t))
	require.NoError(t, err)
	assert.True(t, review.Approved())
	assert.Len(t, *requests, 2)
}

func TestMistralCritic_GivesUp(t *testing.T) {
	srv, requests := criticServer(t, `{"verdicts": [], "voice_score": 7, "voice_problems": ""}`)
	critic, err := NewMistralCritic("test-key", "ministral-8b-2512", srv.URL, 5*time.Second)
	require.NoError(t, err)

	_, err = critic.Review(context.Background(), banksyPost(t), banksyDraft(t))
	assert.ErrorContains(t, err, "voice score out of range")
	assert.Len(t, *requests, criticAttempts)

	_, err = critic.Review(context.Background(), nil, banksyDraft(t))
	assert.Error(t, err)
}

func TestNewMistralCritic_Validation(t *testing.T) {
	_, err := NewMistralCritic("", "ministral-8b-2512", "", time.Second)
	assert.ErrorIs(t, err, ports.ErrAPIKeyInvalid)
	_, err = NewMistralCritic("key", " ", "", time.Second)
	assert.Error(t, err)
}

func TestMistralCritic_TestConnection(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		body    string
		wantErr bool
	}{
		"served":  {http.StatusOK, `{"id":"ministral-8b-2512"}`, false},
		"retired": {http.StatusOK, `{"id":"ministral-8b-2512","deprecation":"2020-01-01T00:00:00Z"}`, true},
		"missing": {http.StatusNotFound, `{"message":"not found"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/models/ministral-8b-2512", r.URL.Path)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			critic, err := NewMistralCritic("test-key", "ministral-8b-2512", srv.URL, 5*time.Second)
			require.NoError(t, err)
			err = critic.TestConnection(context.Background())
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
