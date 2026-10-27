package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReview_Approved(t *testing.T) {
	supported := ClaimVerdict{Claim: "Banksy painted the wall in 2003.", Supported: true, Evidence: "painted during the Bach Jumps festival in 2003"}
	invented := ClaimVerdict{Claim: "The wall is red.", Supported: false, Evidence: "the story names no colour"}

	assert.True(t, Review{Verdicts: []ClaimVerdict{supported}, VoiceScore: VoiceScoreGood}.Approved())
	assert.True(t, Review{VoiceScore: VoiceScoreFair}.Approved(), "a draft without claims can pass")
	assert.True(t, Review{Verdicts: []ClaimVerdict{supported}}.Approved(), "an unscored voice does not block")
	assert.False(t, Review{Verdicts: []ClaimVerdict{supported, invented}, VoiceScore: VoiceScoreGood}.Approved())
	assert.False(t, Review{Verdicts: []ClaimVerdict{supported}, VoiceScore: VoiceScorePoor}.Approved())
}

func TestReview_Feedback(t *testing.T) {
	review := Review{
		Verdicts: []ClaimVerdict{
			{Claim: "Banksy painted the wall in 2003.", Supported: true},
			{Claim: "The wall is red.", Supported: false, Evidence: "the story names no colour"},
			{Claim: "Thousands came to see it.", Supported: false},
		},
		VoiceScore:    VoiceScorePoor,
		VoiceProblems: "asks the reader a question",
	}
	assert.Equal(t, strings.Join([]string{
		`- Not supported by THE STORY: "The wall is red." (the story names no colour)`,
		`- Not supported by THE STORY: "Thousands came to see it."`,
		`- Voice: asks the reader a question`,
	}, "\n"), review.Feedback())
	assert.Len(t, review.Unsupported(), 2)

	assert.Equal(t, "- Voice: the text breaks the voice rules", Review{VoiceScore: VoiceScorePoor}.Feedback())
	assert.Empty(t, Review{VoiceScore: VoiceScoreGood}.Feedback())
}

func TestFallbackContent(t *testing.T) {
	banksy := postWithArticle(t, Article{
		Title:   "Banksy Revealed!",
		Section: "Street Art",
		Tags: Tags{
			Artists:    []string{"Banksy", "Brad Downey"},
			Places:     []string{"Berlin"},
			Techniques: []string{"Mural Art"},
		},
	})

	bluesky, err := FallbackContent(banksy, PlatformBluesky)
	require.NoError(t, err)
	assert.True(t, bluesky.IsFallback())
	assert.Equal(t, "Banksy Revealed!\nBanksy and Brad Downey in Berlin, September 2011.", bluesky.Text())
	assert.Equal(t, []string{"#Banksy", "#Berlin", "#MuralArt"}, bluesky.Hashtags())
	assert.Equal(t, banksy.URI(), bluesky.URI())
	assert.Empty(t, bluesky.Claims())

	mastodon, err := FallbackContent(banksy, PlatformMastodon)
	require.NoError(t, err)
	assert.Equal(t, "Banksy Revealed!\nBanksy and Brad Downey in Berlin, September 2011.\n\nhttps://www.urbanartcore.com/banksy-revealed/", mastodon.Text())
}

func TestFallbackContent_SparseArticles(t *testing.T) {
	tests := []struct {
		name    string
		article Article
		want    string
	}{
		{"nothing known: title from the URL", Article{}, "Banksy Revealed\nSeptember 2011."},
		{"place only", Article{Title: "A Wall", Tags: Tags{Places: []string{"Paris"}}}, "A Wall\nParis, September 2011."},
		{"artist only", Article{Title: "A Wall", Tags: Tags{Artists: []string{"Roa"}}}, "A Wall\nRoa, September 2011."},
		{"many artists", Article{Title: "A Show", Tags: Tags{Artists: []string{"Os Gemeos", "Blu", "JR"}}}, "A Show\nOs Gemeos, Blu and others, September 2011."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := FallbackContent(postWithArticle(t, tt.article), PlatformBluesky)
			require.NoError(t, err)
			assert.Equal(t, tt.want, content.Text())
		})
	}

	long, err := FallbackContent(postWithArticle(t, Article{Title: strings.Repeat("Very long title ", 40)}), PlatformBluesky)
	require.NoError(t, err, "a long title is cut to fit")
	assert.LessOrEqual(t, long.CharacterCount(), BlueskyCharacterLimit)

	_, err = FallbackContent(nil, PlatformBluesky)
	assert.Error(t, err)
}

func TestGeneratedContent_Claims(t *testing.T) {
	content, err := NewGeneratedContent("Banksy painted a wall in Berlin.", PlatformBluesky, nil, "prompt", "https://www.urbanartcore.com/banksy-revealed/")
	require.NoError(t, err)
	assert.False(t, content.IsFallback())

	content.WithClaims([]string{" Banksy painted a wall. ", "", "It is in Berlin."})
	claims := content.Claims()
	assert.Equal(t, []string{"Banksy painted a wall.", "It is in Berlin."}, claims)
	claims[0] = "changed"
	assert.Equal(t, "Banksy painted a wall.", content.Claims()[0], "Claims returns a copy")

	var nilContent *GeneratedContent
	assert.Nil(t, nilContent.WithClaims([]string{"x"}))
	assert.False(t, nilContent.IsFallback())
}
