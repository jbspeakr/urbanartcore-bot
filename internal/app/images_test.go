package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// fakeDescriber returns a fixed description or error and records the image.
type fakeDescriber struct {
	description string
	err         error
	images      []string
}

func (d *fakeDescriber) Describe(ctx context.Context, post *domain.Post, imageURL string) (string, error) {
	d.images = append(d.images, imageURL)
	return d.description, d.err
}

func (d *fakeDescriber) TestConnection(ctx context.Context) error { return nil }

func withCover(post *domain.Post) *domain.Post {
	return post.WithArticle(domain.Article{
		Title:  "Graffiti meets Thalys in Paris",
		Credit: "Ian Cox",
		Cover:  domain.Cover{URL: "https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/train.jpg", Alt: "Graffiti meets Thalys in Paris"},
	})
}

func TestImages_DescribedCoverOnEveryPlatform(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, withCover(thalys))
	describer := &fakeDescriber{description: "A silver high-speed train covered in pink letters"}
	rig.service.WithImageDescriber(describer)

	resp := rig.publish(t, true)
	require.Len(t, resp.Generated, 2)
	for platform, content := range resp.Generated {
		images := content.Images()
		require.Len(t, images, 1, platform)
		assert.Equal(t, "https://www.urbanartcore.com/graffiti-meets-thalys-in-paris/train.jpg", images[0].URL)
		assert.Equal(t, "A silver high-speed train covered in pink letters. Photo: Ian Cox.", images[0].Alt)
	}
	assert.Len(t, describer.images, 1, "the cover is described once for all platforms")
}

func TestImages_SiteAltTextWhenDescriberFails(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, withCover(thalys))
	rig.service.WithImageDescriber(&fakeDescriber{err: errors.New("503")})

	resp := rig.publish(t, true)
	images := resp.Generated[domain.PlatformMastodon].Images()
	require.Len(t, images, 1)
	assert.Equal(t, "Graffiti meets Thalys in Paris. Photo: Ian Cox.", images[0].Alt)
}

func TestImages_NoCoverNoImage(t *testing.T) {
	rig := newLedgerRig(t, &memLedger{}, thalys)
	describer := &fakeDescriber{description: "unused"}
	rig.service.WithImageDescriber(describer)

	resp := rig.publish(t, true)
	assert.Empty(t, resp.Generated[domain.PlatformMastodon].Images())
	assert.Empty(t, describer.images)
}
