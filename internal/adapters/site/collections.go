package site

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// maxCollectionBytes caps how much of a collection's index.json or of
// llms.txt is read.
const maxCollectionBytes = 1 << 20

// knownCollections are the collections the site listed when this was
// written, used when llms.txt cannot be read.
var knownCollections = []string{
	"abandoned-places", "adbusting", "banksy-years", "blu-and-roa", "bristol",
	"interviews", "light-and-projection", "stencil-art", "tape-yarn-cardboard", "trains-and-tunnels",
}

var collectionLink = regexp.MustCompile(`/collections/([a-z0-9-]+)/`)

// collectionJSON is a collection's index.json.
type collectionJSON struct {
	Title       string `json:"title"`
	Kicker      string `json:"kicker"`
	Description string `json:"description"`
	Intro       string `json:"intro"`
	URL         string `json:"url"`
	Items       []struct {
		Position int    `json:"position"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Note     string `json:"note"`
		Date     string `json:"date"`
	} `json:"items"`
}

// Collections returns the site's collections, found through llms.txt, with
// their items in reading order. A collection that cannot be read is left
// out; none at all is an error.
func (c *Client) Collections(ctx context.Context) ([]domain.Collection, error) {
	slugs := c.collectionSlugs(ctx)
	collections := make([]domain.Collection, 0, len(slugs))
	var lastErr error
	for _, slug := range slugs {
		var raw collectionJSON
		uri := c.baseURL + "/collections/" + slug + "/index.json"
		if err := c.getJSON(ctx, uri, maxCollectionBytes, &raw); err != nil {
			lastErr = err
			slog.Default().Warn("Could not read a collection", slog.String("url", uri), slog.String("error", err.Error()))
			continue
		}
		collection := raw.domain(slug, c.baseURL)
		if len(collection.Items) == 0 {
			continue
		}
		collections = append(collections, collection)
	}
	if len(collections) == 0 {
		return nil, ports.RepositoryError{Err: fmt.Errorf("%w: no collection could be read: %v", ports.ErrDataSourceNotFound, lastErr)}
	}
	return collections, nil
}

// collectionSlugs reads the collection links from llms.txt, falling back to
// knownCollections.
func (c *Client) collectionSlugs(ctx context.Context) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/llms.txt", nil)
	if err == nil {
		req.Header.Set("User-Agent", userAgent)
		var resp *http.Response
		if resp, err = c.http.Do(req); err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCollectionBytes))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && readErr == nil {
				seen := make(map[string]bool)
				var slugs []string
				for _, m := range collectionLink.FindAllStringSubmatch(string(body), -1) {
					if !seen[m[1]] {
						seen[m[1]] = true
						slugs = append(slugs, m[1])
					}
				}
				if len(slugs) > 0 {
					return slugs
				}
			}
			err = fmt.Errorf("llms.txt returned status %d with no collection links", resp.StatusCode)
		}
	}
	slog.Default().Warn("Could not list the collections from llms.txt, using the known list", slog.String("error", err.Error()))
	return append([]string(nil), knownCollections...)
}

func (raw collectionJSON) domain(slug, baseURL string) domain.Collection {
	collection := domain.Collection{
		Slug:        slug,
		Title:       clean(raw.Title),
		Kicker:      clean(raw.Kicker),
		Description: clean(raw.Description),
		Intro:       clean(raw.Intro),
		URL:         strings.TrimSpace(raw.URL),
	}
	if collection.URL == "" {
		collection.URL = baseURL + "/collections/" + slug + "/"
	}
	for _, item := range raw.Items {
		if strings.TrimSpace(item.URL) == "" || clean(item.Title) == "" {
			continue
		}
		date, _ := time.Parse(domain.DateLayout, item.Date)
		collection.Items = append(collection.Items, domain.CollectionItem{
			Position: item.Position,
			Title:    clean(item.Title),
			URL:      strings.TrimSpace(item.URL),
			Note:     clean(item.Note),
			Date:     date,
		})
	}
	sort.SliceStable(collection.Items, func(i, j int) bool { return collection.Items[i].Position < collection.Items[j].Position })
	return collection
}
