package reader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

const samplePage = `<!doctype html>
<html><head>
<title>Bomb It: Israel | Urban Art Core</title>
<meta property="og:site_name" content="Urban Art Core">
<meta property="og:title" content="Bomb It: Israel | Urban Art Core">
<meta property="og:description" content="Jon Reiss takes Bomb It to Tel Aviv &amp; Jerusalem.">
<script>var x = "<p>not text</p>";</script>
</head><body>
<nav><p>Home</p></nav>
<article>
  <h1>Bomb It: Israel</h1>
  <div class="entry-content">
    <p>The   Bomb It crew   visited Tel Aviv.</p>
    <p></p>
    <p>Writers talk about painting <em>the wall</em>.</p>
  </div>
</article>
<footer><p>© Urban Art Core</p></footer>
</body></html>`

func parse(t *testing.T, page string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	require.NoError(t, err)
	return doc
}

func TestParseArticle(t *testing.T) {
	article := ParseArticle(parse(t, samplePage))

	assert.Equal(t, "Bomb It: Israel", article.Title)
	assert.Equal(t, "Jon Reiss takes Bomb It to Tel Aviv & Jerusalem.", article.Description)
	assert.Equal(t, "The Bomb It crew visited Tel Aviv. Writers talk about painting the wall.", article.Excerpt)
}

func TestParseArticle_FallsBackToTitleTagAndBody(t *testing.T) {
	article := ParseArticle(parse(t, `<html><head><title>Roa in London</title>
<meta name="description" content="A giant rabbit."></head>
<body><p>Roa painted a rabbit in Hackney.</p></body></html>`))

	assert.Equal(t, "Roa in London", article.Title)
	assert.Equal(t, "A giant rabbit.", article.Description)
	assert.Equal(t, "Roa painted a rabbit in Hackney.", article.Excerpt)
}

func TestExcerpt_CutsAtWordBoundary(t *testing.T) {
	got := excerpt([]string{"one two three", "four five six seven"}, 22)
	assert.Equal(t, "one two three four …", got)

	assert.Equal(t, "short", excerpt([]string{"short"}, 100))
	assert.Equal(t, "", excerpt(nil, 100))
}

func TestReadArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bomb-it-israel/":
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			_, _ = w.Write([]byte(samplePage))
		case "/empty/":
			_, _ = w.Write([]byte(`<html><body></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	r := NewWebArticleReader()
	ctx := context.Background()

	article, err := r.ReadArticle(ctx, server.URL+"/bomb-it-israel/")
	require.NoError(t, err)
	assert.Equal(t, "Bomb It: Israel", article.Title)

	_, err = r.ReadArticle(ctx, server.URL+"/missing/")
	assert.ErrorContains(t, err, "404")

	_, err = r.ReadArticle(ctx, server.URL+"/empty/")
	assert.Error(t, err)
}
