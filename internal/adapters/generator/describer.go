package generator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

const (
	// describerMaxTokens bounds a description of one or two sentences.
	describerMaxTokens = 300
	// describerMaxGraphemes is the longest description kept; longer ones are cut.
	describerMaxGraphemes = 400
)

// describerSystemPrompt asks for alt text a screen reader user would want.
const describerSystemPrompt = `You write alt text for photographs of street art and graffiti, for people who cannot see them.

Describe what the photograph shows in one or two plain sentences, at most 300 characters: the artwork, its subject, style and colours, and where it is (a wall, a train, a gallery). Mention people only by what they do.

You get the story the photograph belongs to. You may name the artists and places it lists when the photograph shows their work; name no one else. Do not guess at meaning or intent, and do not read out lettering you cannot see clearly.

Do not start with "Image of", "Photo of" or "Picture of". Answer with the alt text only, no quotes or labels.`

// MistralDescriber implements ports.ImageDescriber with a Mistral model
// that accepts images.
type MistralDescriber struct {
	model  string
	client *mistralClient
}

// NewMistralDescriber creates a describer that calls model with apiKey. An
// empty endpoint means the public Mistral API.
func NewMistralDescriber(apiKey, model, endpoint string, timeout time.Duration) (*MistralDescriber, error) {
	if apiKey == "" {
		return nil, ports.GeneratorError{Model: model, Err: ports.ErrAPIKeyInvalid}
	}
	if strings.TrimSpace(model) == "" {
		return nil, ports.GeneratorError{Model: "unknown", Err: errors.New("vision model cannot be empty")}
	}
	return &MistralDescriber{model: model, client: newMistralClient(apiKey, endpoint, timeout)}, nil
}

// Describe returns alt text for the image at imageURL.
func (d *MistralDescriber) Describe(ctx context.Context, post *domain.Post, imageURL string) (string, error) {
	if post == nil || strings.TrimSpace(imageURL) == "" {
		return "", errors.New("post and image URL are required")
	}
	req := &partsRequest{
		Model: d.model,
		Messages: []partsMessage{
			{Role: roleSystem, Content: []contentPart{{Type: "text", Text: describerSystemPrompt}}},
			{Role: roleUser, Content: []contentPart{
				{Type: "text", Text: describerPrompt(post)},
				{Type: "image_url", ImageURL: imageURL},
			}},
		},
		Temperature: 0.2,
		TopP:        1.0,
		MaxTokens:   describerMaxTokens,
	}

	resp, err := d.client.chatWithParts(ctx, req)
	if err != nil {
		return "", ports.GeneratorError{Model: d.model, Err: err}
	}
	if len(resp.Choices) == 0 {
		return "", ports.GeneratorError{Model: d.model, Err: errors.New("no choices in response")}
	}
	description := cleanDescription(resp.Choices[0].Message.Content)
	if description == "" {
		return "", ports.GeneratorError{Model: d.model, Err: errors.New("empty description")}
	}
	return description, nil
}

// describerPrompt names the story and the tags the description may use.
func describerPrompt(post *domain.Post) string {
	article := post.Article()
	var b strings.Builder
	b.WriteString("THE STORY:\n")
	if article.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", article.Title)
	}
	if article.Description != "" {
		fmt.Fprintf(&b, "Summary: %s\n", article.Description)
	}
	if len(article.Tags.Artists) > 0 {
		fmt.Fprintf(&b, "Artists: %s\n", strings.Join(article.Tags.Artists, ", "))
	}
	if len(article.Tags.Places) > 0 {
		fmt.Fprintf(&b, "Places: %s\n", strings.Join(article.Tags.Places, ", "))
	}
	if len(article.Tags.Techniques) > 0 {
		fmt.Fprintf(&b, "Techniques: %s\n", strings.Join(article.Tags.Techniques, ", "))
	}
	fmt.Fprintf(&b, "Published: %s\n\nWrite the alt text for this photograph.", post.Date().Format("January 2006"))
	return b.String()
}

// cleanDescription strips the labels and quotes models like to add.
func cleanDescription(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	for _, label := range []string{"Alt text:", "Alt-text:", "Alt:"} {
		if len(text) >= len(label) && strings.EqualFold(text[:len(label)], label) {
			text = strings.TrimSpace(text[len(label):])
		}
	}
	text = strings.Trim(text, "\"'“”")
	if domain.CountGraphemes(text) > describerMaxGraphemes {
		text = domain.TruncateWords(text, describerMaxGraphemes/6)
	}
	return strings.TrimSpace(text)
}

// TestConnection verifies the model is served and accepts images.
func (d *MistralDescriber) TestConnection(ctx context.Context) error {
	card, err := d.client.getModel(ctx, d.model)
	if err != nil {
		return ports.GeneratorError{Model: d.model, Err: err}
	}
	if !card.Capabilities.Vision {
		return ports.GeneratorError{Model: d.model, Err: errors.New("model does not accept images")}
	}
	return nil
}
