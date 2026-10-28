package generator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMistralDescriber_Describe(t *testing.T) {
	var got partsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant",
				"content": "Alt text: \"A stencilled rat holds a paint roller on a white wall.\""}}},
		})
	}))
	defer srv.Close()

	d, err := NewMistralDescriber("test-key", "mistral-large-2512", srv.URL, 5*time.Second)
	require.NoError(t, err)
	description, err := d.Describe(context.Background(), banksyPost(t), "https://www.urbanartcore.com/banksy-revealed/wall.jpg")
	require.NoError(t, err)
	assert.Equal(t, "A stencilled rat holds a paint roller on a white wall.", description, "labels and quotes are stripped")

	assert.Equal(t, "mistral-large-2512", got.Model)
	require.Len(t, got.Messages, 2)
	assert.Contains(t, got.Messages[0].Content[0].Text, "alt text")
	user := got.Messages[1].Content
	require.Len(t, user, 2)
	assert.Contains(t, user[0].Text, "Artists: Banksy, Brad Downey")
	assert.Contains(t, user[0].Text, "Places: Berlin")
	assert.Equal(t, "image_url", user[1].Type)
	assert.Equal(t, "https://www.urbanartcore.com/banksy-revealed/wall.jpg", user[1].ImageURL)
}

func TestMistralDescriber_Errors(t *testing.T) {
	_, err := NewMistralDescriber("", "m", "", time.Second)
	assert.Error(t, err)
	_, err = NewMistralDescriber("k", " ", "", time.Second)
	assert.Error(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "  \"\" "}}},
		})
	}))
	defer srv.Close()
	d, err := NewMistralDescriber("k", "m", srv.URL, time.Second)
	require.NoError(t, err)
	_, err = d.Describe(context.Background(), banksyPost(t), "https://x/y.jpg")
	assert.ErrorContains(t, err, "empty description")
	_, err = d.Describe(context.Background(), banksyPost(t), "")
	assert.Error(t, err)
}

func TestMistralDescriber_TestConnection(t *testing.T) {
	for model, wantErr := range map[string]bool{"pixtral": false, "text-only": true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": model, "capabilities": map[string]any{"vision": model == "pixtral"}})
		}))
		d, err := NewMistralDescriber("k", model, srv.URL, time.Second)
		require.NoError(t, err)
		err = d.TestConnection(context.Background())
		if wantErr {
			assert.ErrorContains(t, err, "does not accept images", model)
		} else {
			assert.NoError(t, err, model)
		}
		srv.Close()
	}
}

func TestCleanDescription(t *testing.T) {
	assert.Equal(t, "A wall.", cleanDescription("alt: “A   wall.”"))
	long := cleanDescription(repeatWords("paint", 200))
	assert.Less(t, len(long), 600)
}

func repeatWords(w string, n int) string {
	out := w
	for i := 1; i < n; i++ {
		out += " " + w
	}
	return out
}
