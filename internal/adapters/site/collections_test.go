package site

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bristolJSON = `{
 "description": "Three reports on See No Evil.",
 "intro": "Follow See No Evil.",
 "items": [
  {"date": "2012-08-22", "note": "The finished walls.", "position": 2, "title": "See No Evil Bristol - Final Walls", "url": "https://www.urbanartcore.com/see-no-evil-bristol-final-walls/"},
  {"date": "2011-10-17", "note": "The 2011 edition.", "position": 1, "title": "Europe’s Largest Street Art Event", "url": "https://www.urbanartcore.com/europe-largest-street-art-event/"},
  {"date": "2012-01-01", "note": "No address.", "position": 3, "title": "Broken", "url": ""}
 ],
 "kicker": "Bristol, 2011–2012",
 "title": "One street, two summers",
 "type": "collection",
 "url": "https://www.urbanartcore.com/collections/bristol/"
}`

func TestCollections_FromLLMSTxt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/llms.txt":
			_, _ = io.WriteString(w, "## Collections\n- [One street](https://www.urbanartcore.com/collections/bristol/) (3 posts)\n- [Gone](https://www.urbanartcore.com/collections/gone/)\n- [Again](https://www.urbanartcore.com/collections/bristol/)\n")
		case "/collections/bristol/index.json":
			_, _ = io.WriteString(w, bristolJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	collections, err := client.Collections(context.Background())
	require.NoError(t, err)
	require.Len(t, collections, 1, "a collection that cannot be read is left out")
	c := collections[0]
	assert.Equal(t, "bristol", c.Slug)
	assert.Equal(t, "One street, two summers", c.Title)
	assert.Equal(t, "https://www.urbanartcore.com/collections/bristol/", c.URL)
	require.Len(t, c.Items, 2, "an item without an address is skipped")
	assert.Equal(t, "Europe’s Largest Street Art Event", c.Items[0].Title, "items in reading order")
	assert.Equal(t, 2011, c.Items[0].Date.Year())
	assert.Equal(t, "The finished walls.", c.Items[1].Note)
}

func TestCollections_KnownListWhenLLMSTxtFails(t *testing.T) {
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	_, err = client.Collections(context.Background())
	assert.Error(t, err, "no collection at all is an error")
	assert.Contains(t, requested, "/collections/trains-and-tunnels/index.json")
}

func TestArtistsAndAllPosts(t *testing.T) {
	srv := newFakeSite(t)
	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	ctx := context.Background()

	posts, err := client.AllPosts(ctx)
	require.NoError(t, err)
	require.Len(t, posts, 3)
	assert.True(t, posts[0].Date().Before(posts[2].Date()), "oldest first")

	artists, err := client.Artists(ctx)
	require.NoError(t, err)
	byName := map[string]int{}
	for _, a := range artists {
		byName[a.Name] = len(a.Posts)
		if a.Name == "Banksy" {
			assert.Equal(t, srv.URL+"/banksy/", a.URL, "tag paths resolve against the site")
		}
	}
	assert.Equal(t, 1, byName["Banksy"])
	assert.Equal(t, 1, byName["Sainer"])
}
