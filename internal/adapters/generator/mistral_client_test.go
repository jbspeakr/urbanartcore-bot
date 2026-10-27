package generator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// TestGenerate_SendsJSONSchema checks every chat request asks for structured
// output against the draft schema, with the voice in the system message.
func TestGenerate_SendsJSONSchema(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "test",
			"model": got.Model,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": `{"text":"In September 2009 a Thalys train in Paris was painted."}`},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	config := createTestConfig()
	config.Model = "mistral-large-2512"
	config.Endpoint = server.URL
	adapter, err := NewMistralAdapter(config, &mockPromptManager{})
	require.NoError(t, err)

	req, err := ports.NewGenerationRequest([]*domain.Post{createTestPost()}, domain.PlatformBluesky, "09-14")
	require.NoError(t, err)

	content, err := adapter.GenerateContent(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, createTestPost().Hashtags(), content.Hashtags(), "hashtags come from the post's tags")

	assert.Equal(t, "mistral-large-2512", got.Model)
	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	assert.True(t, got.ResponseFormat.JSONSchema.Strict)
	assert.Equal(t, []any{"text"}, got.ResponseFormat.JSONSchema.Schema["required"])
	require.Len(t, got.Messages, 2)
	assert.Equal(t, roleSystem, got.Messages[0].Role)
	assert.Contains(t, got.Messages[0].Content, "third person")
}

// TestTestConnection_ModelCheck checks the startup check against the models endpoint.
func TestTestConnection_ModelCheck(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		expectErr error
	}{
		{name: "model served", status: http.StatusOK, body: `{"id":"mistral-large-2512"}`},
		{name: "model scheduled for retirement", status: http.StatusOK, body: `{"id":"mistral-large-2512","deprecation":"2999-01-01T00:00:00Z"}`},
		{name: "model retired", status: http.StatusOK, body: `{"id":"mistral-large-2512","deprecation":"2020-01-01T00:00:00Z"}`, expectErr: ports.ErrModelNotFound},
		{name: "model unknown", status: http.StatusNotFound, body: `{"detail":"Model not found"}`, expectErr: ports.ErrModelNotFound},
		{name: "bad key", status: http.StatusUnauthorized, body: `{"message":"Unauthorized"}`, expectErr: ports.ErrAPIKeyInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models/mistral-large-2512", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			config := createTestConfig()
			config.Model = "mistral-large-2512"
			config.Endpoint = server.URL
			config.Timeout = 5 * time.Second
			adapter, err := NewMistralAdapter(config, &mockPromptManager{})
			require.NoError(t, err)

			err = adapter.TestConnection(context.Background())
			if tt.expectErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectErr), "got %v", err)
		})
	}
}
