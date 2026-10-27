package reader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

const (
	// defaultTimeout bounds one page fetch; the post is short HTML.
	defaultTimeout = 15 * time.Second
	// maxPageBytes caps how much of a page is read.
	maxPageBytes = 2 << 20
	// maxExcerptChars caps the body text handed to the model.
	maxExcerptChars = 1500
	// userAgent identifies the bot to the blog's web server.
	userAgent = "uac-socializer/2 (+https://www.urbanartcore.com)"
)

// WebArticleReader implements ports.ArticleReader over plain HTTP.
type WebArticleReader struct {
	client *http.Client
}

// NewWebArticleReader creates a reader with a bounded HTTP client.
func NewWebArticleReader() *WebArticleReader {
	return &WebArticleReader{client: &http.Client{Timeout: defaultTimeout}}
}

// ReadArticle fetches uri and extracts its title, description and opening paragraphs.
func (r *WebArticleReader) ReadArticle(ctx context.Context, uri string) (*ports.Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", uri, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxPageBytes))
		return nil, fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, uri)
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	article := ParseArticle(doc)
	if article.Title == "" && article.Description == "" && article.Excerpt == "" {
		return nil, fmt.Errorf("no title, description or text found at %s", uri)
	}
	return article, nil
}

// ParseArticle extracts the title, description and opening paragraphs
// from a parsed blog post page.
func ParseArticle(doc *html.Node) *ports.Article {
	meta := map[string]string{}
	var titleTag string
	var body *html.Node

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				key := strings.ToLower(attr(n, "property"))
				if key == "" {
					key = strings.ToLower(attr(n, "name"))
				}
				if _, seen := meta[key]; key != "" && !seen {
					meta[key] = clean(attr(n, "content"))
				}
			case "title":
				if titleTag == "" {
					titleTag = clean(textOf(n))
				}
			case "article":
				if body == nil {
					body = n
				}
			default:
				if body == nil && hasClass(n, "entry-content") {
					body = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	article := &ports.Article{
		Title:       stripSiteName(firstNonEmpty(meta["og:title"], meta["twitter:title"], titleTag), meta["og:site_name"]),
		Description: firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"]),
	}
	if body == nil {
		body = doc
	}
	article.Excerpt = excerpt(paragraphs(body), maxExcerptChars)
	return article
}

// paragraphs returns the non-empty text of every <p> below n.
func paragraphs(n *html.Node) []string {
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "nav", "footer", "aside", "form":
				return
			case "p":
				if text := clean(textOf(n)); text != "" {
					out = append(out, text)
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// excerpt joins paragraphs until limit characters, cutting the last one
// at a word boundary.
func excerpt(paras []string, limit int) string {
	var b strings.Builder
	for _, p := range paras {
		sep := 0
		if b.Len() > 0 {
			sep = 1
		}
		room := limit - utf8.RuneCountInString(b.String()) - sep
		if room <= 0 {
			break
		}
		if sep == 1 {
			b.WriteString(" ")
		}
		if utf8.RuneCountInString(p) <= room {
			b.WriteString(p)
			continue
		}
		cut := []rune(p)[:room]
		if i := strings.LastIndex(string(cut), " "); i > 0 {
			cut = []rune(string(cut)[:i])
		}
		b.WriteString(string(cut) + " …")
		break
	}
	return b.String()
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// clean collapses runs of whitespace; the HTML parser already decoded entities.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stripSiteName drops a trailing " | Site" or " - Site" from a page title.
func stripSiteName(title, site string) string {
	if site == "" {
		return title
	}
	for _, sep := range []string{" | ", " - ", " – ", " — "} {
		if trimmed, ok := strings.CutSuffix(title, sep+site); ok {
			return trimmed
		}
	}
	return title
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
