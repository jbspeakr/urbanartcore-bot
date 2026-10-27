package domain

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MaxPostHashtags is how many hashtags a post carries on every platform.
const MaxPostHashtags = 3

// Generic hashtags fill the slots the post's own tags leave open.
const (
	HashtagStreetArt = "#StreetArt"
	HashtagGraffiti  = "#Graffiti"
)

// Hashtags derives the post's hashtags from its typed tags, so they name the
// artist, the place and the technique rather than a fixed generic set. The
// first artist, place, technique and format come first, then the remaining
// artists, places and techniques, then #StreetArt and #Graffiti, the one
// matching the post's section first. The result always holds MaxPostHashtags
// distinct tags in CamelCase, which screen readers read word by word.
func (p *Post) Hashtags() []string {
	article := p.Article()
	tags := article.Tags

	var candidates []string
	candidates = append(candidates, first(tags.Artists), first(tags.Places), first(tags.Techniques), first(tags.Formats))
	candidates = append(candidates, rest(tags.Artists)...)
	candidates = append(candidates, rest(tags.Places)...)
	candidates = append(candidates, rest(tags.Techniques)...)
	if strings.EqualFold(strings.TrimSpace(article.Section), "graffiti") {
		candidates = append(candidates, HashtagGraffiti, HashtagStreetArt)
	} else {
		candidates = append(candidates, HashtagStreetArt, HashtagGraffiti)
	}

	hashtags := make([]string, 0, MaxPostHashtags)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		tag := HashtagFromLabel(candidate)
		if tag == "" || seen[strings.ToLower(tag)] {
			continue
		}
		seen[strings.ToLower(tag)] = true
		hashtags = append(hashtags, tag)
		if len(hashtags) == MaxPostHashtags {
			break
		}
	}
	return hashtags
}

// HashtagFromLabel turns a tag label into a CamelCase hashtag, for example
// "Brad Downey" into "#BradDowney" and "Zürich" into "#Zurich". Accents are
// dropped and other characters split words. It returns "" for a label that
// leaves fewer than two characters or no letter, which no platform links.
func HashtagFromLabel(label string) string {
	label = strings.TrimPrefix(strings.TrimSpace(label), "#")
	var b strings.Builder
	hasLetter := false
	for _, word := range strings.FieldsFunc(stripAccents(label), func(r rune) bool {
		return r > unicode.MaxASCII || !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		b.WriteString(strings.ToUpper(word[:1]) + word[1:])
		if strings.IndexFunc(word, unicode.IsLetter) >= 0 {
			hasLetter = true
		}
	}
	if b.Len() < MinHashtagLength || b.Len() > MaxHashtagLength || !hasLetter {
		return ""
	}
	return "#" + b.String()
}

// stripAccents decomposes s and drops the combining marks.
func stripAccents(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(s))
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func rest(values []string) []string {
	if len(values) < 2 {
		return nil
	}
	return values[1:]
}
