// Package site reads the Urban Art Core archive from the JSON the site
// publishes: index.json lists every post with its title, teaser and typed
// tags, and every post has its own index.json with the full article.
package site

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

const (
	// DefaultBaseURL is the archive's home.
	DefaultBaseURL = "https://www.urbanartcore.com"
	// defaultTimeout bounds one request; the catalogue is about a megabyte.
	defaultTimeout = 20 * time.Second
	// maxCatalogueBytes caps how much of index.json is read.
	maxCatalogueBytes = 32 << 20
	// maxPostBytes caps how much of a post's index.json is read.
	maxPostBytes = 8 << 20
	// userAgent names the bot and its repository, as the site's llms.txt asks.
	userAgent = "urbanartcore-bot (+https://github.com/jbspeakr/urbanartcore-bot)"
)

// Catalogue sources, reported by Source.
const (
	SourceLive     = "live"
	SourceSnapshot = "snapshot"
)

// snapshot is a copy of the site's index.json built into the binary, used
// when the live catalogue cannot be fetched. `just snapshot` refreshes it.
//
//go:embed snapshot/index.json
var snapshot []byte

var dateKeyPattern = regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)

// Client implements ports.PostRepository and ports.PostReader over the
// site's JSON. The catalogue is loaded on first use, from the live site or,
// failing that, from the built-in snapshot.
type Client struct {
	baseURL  string
	http     *http.Client
	snapshot []byte
	intn     func(n int) int

	once    sync.Once
	loadErr error
	byDate  map[string][]*domain.Post
	count   int
	source  string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.http = client }
}

// WithSnapshot replaces the built-in catalogue snapshot.
func WithSnapshot(data []byte) Option {
	return func(c *Client) { c.snapshot = data }
}

// WithRandom replaces the function that picks among a day's posts.
func WithRandom(intn func(n int) int) Option {
	return func(c *Client) { c.intn = intn }
}

// NewClient creates a client for the site at baseURL. It makes no request
// until the catalogue is first needed.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, ports.RepositoryError{Err: fmt.Errorf("site base URL must start with http:// or https://, got %q", baseURL)}
	}
	c := &Client{
		baseURL:  baseURL,
		http:     &http.Client{Timeout: defaultTimeout},
		snapshot: snapshot,
		intn:     rand.Intn,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// GetPostForDate returns one of the posts published on dateKey (MM-DD),
// picked at random, with the title, teaser and tags the catalogue holds.
func (c *Client) GetPostForDate(ctx context.Context, dateKey string) (*domain.Post, error) {
	posts, err := c.GetPostsForDate(ctx, dateKey)
	if err != nil {
		return nil, err
	}
	return posts[c.intn(len(posts))], nil
}

// GetPostsForDate returns every post published on dateKey (MM-DD), with the
// title, teaser and tags the catalogue holds.
func (c *Client) GetPostsForDate(ctx context.Context, dateKey string) ([]*domain.Post, error) {
	dateKey = strings.TrimSpace(dateKey)
	if !dateKeyPattern.MatchString(dateKey) {
		return nil, ports.RepositoryError{Err: fmt.Errorf("%w: expected MM-DD format, got %q", ports.ErrInvalidDateFormat, dateKey)}
	}
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	posts := c.byDate[dateKey]
	if len(posts) == 0 {
		return nil, ports.RepositoryError{Err: fmt.Errorf("%w: %s", ports.ErrDateNotFound, dateKey)}
	}
	return append([]*domain.Post(nil), posts...), nil
}

// ListAvailableDates returns every MM-DD key with at least one post, in order.
func (c *Client) ListAvailableDates(ctx context.Context) ([]string, error) {
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	dates := make([]string, 0, len(c.byDate))
	for key := range c.byDate {
		dates = append(dates, key)
	}
	sort.Strings(dates)
	return dates, nil
}

// GetPostCount returns the number of posts in the catalogue.
func (c *Client) GetPostCount(ctx context.Context) (int, error) {
	if err := c.load(ctx); err != nil {
		return 0, err
	}
	return c.count, nil
}

// ValidateDataSource loads the catalogue and fails when neither the live
// site nor the snapshot yields a usable one.
func (c *Client) ValidateDataSource(ctx context.Context) error {
	return c.load(ctx)
}

// Source reports where the catalogue came from: SourceLive or
// SourceSnapshot. It is empty until the catalogue is loaded.
func (c *Client) Source() string {
	return c.source
}

// ReadPost fetches the post's own index.json and returns a copy of post
// carrying the full article. Fields the post's JSON leaves empty keep what
// the catalogue already knew.
func (c *Client) ReadPost(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	if post == nil {
		return nil, errors.New("post cannot be nil")
	}
	uri := strings.TrimRight(post.URI(), "/") + "/index.json"

	var raw postJSON
	if err := c.getJSON(ctx, uri, maxPostBytes, &raw); err != nil {
		return nil, err
	}

	article := post.Article()
	article.Title = firstNonEmpty(clean(raw.Title), article.Title)
	article.Description = firstNonEmpty(clean(raw.Description), article.Description)
	article.Section = firstNonEmpty(clean(raw.Section), article.Section)
	article.Body = firstNonEmpty(PlainText(raw.Markdown), article.Body)
	article.Credit = clean(raw.Credit)
	if tags := raw.tags(); !tags.empty() {
		article.Tags = tags.domain()
	}
	if raw.Cover != nil && raw.Cover.Src != "" {
		article.Cover = domain.Cover{
			URL:    raw.Cover.Src,
			Alt:    clean(raw.Cover.Alt),
			Width:  raw.Cover.Width,
			Height: raw.Cover.Height,
		}
	}
	if article.IsZero() {
		return nil, fmt.Errorf("no title, teaser or text in %s", uri)
	}
	return post.WithArticle(article), nil
}

// load builds the catalogue once, from the live site or the snapshot.
func (c *Client) load(ctx context.Context) error {
	c.once.Do(func() {
		var entries []indexEntry
		liveErr := c.getJSON(ctx, c.baseURL+"/index.json", maxCatalogueBytes, &entries)
		if liveErr == nil {
			liveErr = c.index(entries)
		}
		if liveErr == nil {
			c.source = SourceLive
			slog.Default().Info("Loaded the catalogue from the site",
				slog.String("url", c.baseURL+"/index.json"),
				slog.Int("posts", c.count))
			return
		}

		slog.Default().Warn("Could not load the live catalogue, using the built-in snapshot",
			slog.String("url", c.baseURL+"/index.json"),
			slog.String("error", liveErr.Error()))
		entries = nil
		err := json.Unmarshal(c.snapshot, &entries)
		if err == nil {
			err = c.index(entries)
		}
		if err != nil {
			c.loadErr = ports.RepositoryError{Err: fmt.Errorf("%w: live catalogue: %v; snapshot: %v", ports.ErrDataSourceNotFound, liveErr, err)}
			return
		}
		c.source = SourceSnapshot
		slog.Default().Info("Loaded the catalogue from the snapshot", slog.Int("posts", c.count))
	})
	return c.loadErr
}

// index groups the catalogue's posts by MM-DD. An entry that is not a valid
// post is skipped; a catalogue without any valid post is an error.
func (c *Client) index(entries []indexEntry) error {
	byDate := make(map[string][]*domain.Post)
	count, skipped := 0, 0
	for _, entry := range entries {
		post, err := entry.post()
		if err != nil {
			skipped++
			continue
		}
		key := post.AnniversaryDateKey()
		byDate[key] = append(byDate[key], post)
		count++
	}
	if count == 0 {
		return fmt.Errorf("%w: no valid posts among %d entries", ports.ErrDataCorrupted, len(entries))
	}
	if skipped > 0 {
		slog.Default().Warn("Skipped catalogue entries that are not valid posts", slog.Int("skipped", skipped))
	}
	c.byDate, c.count = byDate, count
	return nil
}

// getJSON fetches uri and decodes at most limit bytes of JSON into v.
func (c *Client) getJSON(ctx context.Context, uri string, limit int64, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", uri, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, limit))
		return fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, uri)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", uri, err)
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("%s is larger than %d bytes", uri, limit)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %s is not the expected JSON: %v", ports.ErrDataCorrupted, uri, err)
	}
	return nil
}

// indexEntry is one post in the site's index.json.
type indexEntry struct {
	Permalink string       `json:"permalink"`
	Date      string       `json:"date"`
	Title     string       `json:"title"`
	Summary   string       `json:"summary"`
	Topics    []indexTopic `json:"topics"`
	Types     []indexTopic `json:"types"`
}

// indexTopic is a typed tag in index.json; types (sections) carry no entity.
type indexTopic struct {
	Entity string `json:"entity"`
	Label  string `json:"label"`
}

func (e indexEntry) post() (*domain.Post, error) {
	date := e.Date
	if len(date) > len(domain.DateLayout) {
		date = date[:len(domain.DateLayout)]
	}
	post, err := domain.NewPost(e.Permalink, date)
	if err != nil {
		return nil, err
	}
	var tags tagLists
	for _, topic := range e.Topics {
		tags.add(topic.Entity, clean(topic.Label))
	}
	article := domain.Article{
		Title:       clean(e.Title),
		Description: clean(e.Summary),
		Tags:        tags.domain(),
	}
	if len(e.Types) > 0 {
		article.Section = clean(e.Types[0].Label)
	}
	return post.WithArticle(article), nil
}

// postJSON is a post's own index.json.
type postJSON struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Section     string     `json:"section"`
	Markdown    string     `json:"markdown"`
	Credit      string     `json:"credit"`
	Cover       *coverJSON `json:"cover"`
	Artists     []string   `json:"artists"`
	Places      []string   `json:"places"`
	Events      []string   `json:"events"`
	Techniques  []string   `json:"techniques"`
	Formats     []string   `json:"formats"`
	Topics      []string   `json:"topics"`
}

type coverJSON struct {
	Src    string `json:"src"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (p postJSON) tags() tagLists {
	var tags tagLists
	for _, group := range []struct {
		entity string
		labels []string
	}{
		{"artist", p.Artists}, {"place", p.Places}, {"event", p.Events},
		{"technique", p.Techniques}, {"format", p.Formats}, {"topic", p.Topics},
	} {
		for _, label := range group.labels {
			tags.add(group.entity, clean(label))
		}
	}
	return tags
}

// tagLists collects labels by entity, dropping empty and repeated ones.
type tagLists struct {
	artists, places, events, techniques, formats, topics []string
}

func (t *tagLists) add(entity, label string) {
	if label == "" {
		return
	}
	var list *[]string
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "artist":
		list = &t.artists
	case "place":
		list = &t.places
	case "event":
		list = &t.events
	case "technique":
		list = &t.techniques
	case "format":
		list = &t.formats
	default:
		list = &t.topics
	}
	for _, existing := range *list {
		if strings.EqualFold(existing, label) {
			return
		}
	}
	*list = append(*list, label)
}

func (t tagLists) empty() bool {
	return len(t.artists)+len(t.places)+len(t.events)+len(t.techniques)+len(t.formats)+len(t.topics) == 0
}

func (t tagLists) domain() domain.Tags {
	return domain.Tags{
		Artists:    t.artists,
		Places:     t.places,
		Events:     t.events,
		Techniques: t.techniques,
		Formats:    t.formats,
		Topics:     t.topics,
	}
}

var (
	markdownImage     = regexp.MustCompile(`!\[[^\]]*\]\((?:<[^>]*>|[^)]*)\)`)
	markdownLink      = regexp.MustCompile(`\[([^\]]*)\]\((?:<[^>]*>|[^)]*)\)`)
	hugoShortcode     = regexp.MustCompile(`\{\{[<%].*?[>%]\}\}`)
	htmlTag           = regexp.MustCompile(`<[^>]+>`)
	markdownEmphasis  = regexp.MustCompile(`\*\*|__|~~|` + "`")
	markdownLineStart = regexp.MustCompile(`(?m)^[ \t]*(?:#{1,6}|>|[-*+]|\d+\.)[ \t]+`)
	blankLines        = regexp.MustCompile(`\n\s*\n`)
)

// PlainText turns a post body in Markdown into plain paragraphs: images,
// shortcodes and HTML are dropped, links keep their text, and emphasis,
// heading and list markers go. Paragraphs are separated by a blank line.
func PlainText(markdown string) string {
	text := markdownImage.ReplaceAllString(markdown, "")
	text = hugoShortcode.ReplaceAllString(text, "")
	text = markdownLink.ReplaceAllString(text, "$1")
	text = htmlTag.ReplaceAllString(text, "")
	text = markdownEmphasis.ReplaceAllString(text, "")
	text = markdownLineStart.ReplaceAllString(text, "")
	text = html.UnescapeString(text)

	var paragraphs []string
	for _, paragraph := range blankLines.Split(text, -1) {
		if paragraph = clean(paragraph); paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

// clean collapses runs of whitespace.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
