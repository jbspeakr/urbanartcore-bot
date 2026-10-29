package domain

import "time"

// TagPage is a tag of the archive, such as an artist, with its posts oldest first.
type TagPage struct {
	Name  string
	URL   string
	Posts []*Post
}

// Collection is one of the site's hand-picked reading routes: posts in the
// order to read them, each with an editorial note.
type Collection struct {
	Slug        string
	Title       string
	Kicker      string
	Description string
	Intro       string
	URL         string
	Items       []CollectionItem
}

// CollectionItem is one post of a collection.
type CollectionItem struct {
	Position int
	Title    string
	URL      string
	Note     string
	Date     time.Time
}
