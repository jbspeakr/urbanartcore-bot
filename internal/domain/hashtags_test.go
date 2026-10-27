package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postWithArticle(t *testing.T, article Article) *Post {
	t.Helper()
	post, err := NewPost("https://www.urbanartcore.com/banksy-revealed/", "2011-09-14")
	require.NoError(t, err)
	return post.WithArticle(article)
}

func TestHashtags(t *testing.T) {
	tests := []struct {
		name    string
		article Article
		want    []string
	}{
		{
			name: "artist, place and technique first",
			article: Article{Section: "Street Art", Tags: Tags{
				Artists:    []string{"Banksy", "Brad Downey"},
				Places:     []string{"Berlin"},
				Techniques: []string{"Mural Art"},
				Formats:    []string{"Exhibition"},
			}},
			want: []string{"#Banksy", "#Berlin", "#MuralArt"},
		},
		{
			name: "format before the remaining artists",
			article: Article{Tags: Tags{
				Artists: []string{"Sainer", "ETAM"},
				Formats: []string{"Exhibition"},
			}},
			want: []string{"#Sainer", "#Exhibition", "#ETAM"},
		},
		{
			name:    "generic tags fill the open slots, section first",
			article: Article{Section: "Graffiti", Tags: Tags{Places: []string{"Paris"}}},
			want:    []string{"#Paris", "#Graffiti", "#StreetArt"},
		},
		{
			name:    "no tags at all",
			article: Article{},
			want:    []string{"#StreetArt", "#Graffiti"},
		},
		{
			name: "duplicates and unusable labels are skipped",
			article: Article{Tags: Tags{
				Artists: []string{"Street Art", "1UP"},
				Places:  []string{"Zürich", "2011"},
			}},
			want: []string{"#StreetArt", "#Zurich", "#1UP"},
		},
		{
			name:    "topics never become hashtags",
			article: Article{Tags: Tags{Topics: []string{"Art Restoration"}}},
			want:    []string{"#StreetArt", "#Graffiti"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := postWithArticle(t, tt.article).Hashtags()
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), MaxPostHashtags)
			for _, platform := range []string{PlatformBluesky, PlatformMastodon} {
				assert.NoError(t, ValidateHashtags(got, platform), platform)
			}
		})
	}
}

func TestHashtagFromLabel(t *testing.T) {
	for label, want := range map[string]string{
		"Brad Downey":       "#BradDowney",
		"ETAM":              "#ETAM",
		"os gêmeos":         "#OsGemeos",
		"Mural Art":         "#MuralArt",
		"#Banksy":           "#Banksy",
		"Stik / Bristol":    "#StikBristol",
		"Herakut's Giant":   "#HerakutSGiant",
		"2011":              "",
		"X":                 "",
		"東京":                "",
		"":                  "",
		"Ludo (Paris Show)": "#LudoParisShow",
	} {
		assert.Equal(t, want, HashtagFromLabel(label), label)
	}
}

func TestPostArticle(t *testing.T) {
	post := postWithArticle(t, Article{Title: "Banksy Revealed!"})
	assert.Equal(t, "Banksy Revealed!", post.Article().Title)
	assert.False(t, post.Article().IsZero())
	assert.True(t, Article{Section: "Graffiti"}.IsZero(), "a section alone says nothing about the post")

	var nilPost *Post
	assert.Nil(t, nilPost.WithArticle(Article{}))
	assert.True(t, nilPost.Article().IsZero())
}

func TestTruncateWords(t *testing.T) {
	assert.Equal(t, "", TruncateWords("  ", 3))
	assert.Equal(t, "one two three", TruncateWords("one two three", 3))
	assert.Equal(t, "one two\n\nthree …", TruncateWords("one two\n\nthree four five", 3))
}
