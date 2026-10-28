package generator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

const (
	// criticMaxTokens bounds the verdict; a post makes a handful of claims.
	criticMaxTokens = 1500
	// criticAttempts is how often a call that returns no usable verdict is tried.
	criticAttempts = 2
	// criticSourceWords caps the body text the critic reads, as for the writer.
	criticSourceWords = 2000
)

// criticSystemPrompt tells the critic what it checks and how strictly.
const criticSystemPrompt = `You check social media posts against the story they promote, before they are published.

You get THE STORY, THE DRAFT and the CLAIMS its writer listed. Check every claim, and every other factual statement in THE DRAFT that the list leaves out: a name, place, date, number, technique, colour, size, quote or reaction.

A statement is supported only when THE STORY states it or it follows directly from what THE STORY states. The publication date and the tags count as part of THE STORY. The URL at the end of THE DRAFT is the story's own link: it is neither a claim nor a voice problem. Anything else is unsupported, even when it is probably true. Quote the passage of THE STORY that supports a statement as its evidence, or say briefly why nothing does.

Then score THE DRAFT against the voice rules:
3: plain and specific, names people and places from THE STORY, no hype.
2: acceptable, with small lapses.
1: breaks a rule: asks the reader a question, uses emoji or exclamation marks, hypes ("iconic", "stunning", "must-see"), opens with "Check out", "Dive into" or "Discover", or talks about the blog, the archive, the post, an anniversary or years passing.
Name what breaks the rules in voice_problems, or leave it empty.

Respond with a JSON object that matches the given schema.`

// criticSchema is the JSON schema the API enforces on every verdict.
var criticSchema = &responseFormat{
	Type: "json_schema",
	JSONSchema: &jsonSchema{
		Name:   "draft_review",
		Strict: true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verdicts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"claim":     map[string]any{"type": "string"},
							"supported": map[string]any{"type": "boolean"},
							"evidence":  map[string]any{"type": "string"},
						},
						"required":             []string{"claim", "supported", "evidence"},
						"additionalProperties": false,
					},
				},
				"voice_score":    map[string]any{"type": "integer", "description": "1, 2 or 3"},
				"voice_problems": map[string]any{"type": "string"},
			},
			"required":             []string{"verdicts", "voice_score", "voice_problems"},
			"additionalProperties": false,
		},
	},
}

// MistralCritic implements ports.Critic with a small Mistral model.
type MistralCritic struct {
	model  string
	client *mistralClient
}

// NewMistralCritic creates a critic that calls model with apiKey. An empty
// endpoint means the public Mistral API.
func NewMistralCritic(apiKey, model, endpoint string, timeout time.Duration) (*MistralCritic, error) {
	if apiKey == "" {
		return nil, ports.GeneratorError{Model: model, Err: ports.ErrAPIKeyInvalid}
	}
	if strings.TrimSpace(model) == "" {
		return nil, ports.GeneratorError{Model: "unknown", Err: errors.New("critic model cannot be empty")}
	}
	return &MistralCritic{model: model, client: newMistralClient(apiKey, endpoint, timeout)}, nil
}

// Review checks content against post's article. It returns an error when
// the model gives no usable verdict; the caller then treats the draft as
// unchecked.
func (c *MistralCritic) Review(ctx context.Context, post *domain.Post, content *domain.GeneratedContent) (*domain.Review, error) {
	if post == nil || content == nil {
		return nil, errors.New("post and content are required")
	}

	chatReq := &chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: roleSystem, Content: criticSystemPrompt},
			{Role: roleUser, Content: criticPrompt(post, content)},
		},
		Temperature:    0,
		TopP:           1.0,
		MaxTokens:      criticMaxTokens,
		ResponseFormat: criticSchema,
	}

	var lastErr error
	for attempt := 1; attempt <= criticAttempts; attempt++ {
		review, err := c.review(ctx, chatReq)
		if err == nil {
			return review, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		slog.Default().Warn("Critic call failed",
			slog.String("model", c.model),
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()))
	}
	return nil, ports.GeneratorError{Model: c.model, Err: fmt.Errorf("no usable review: %w", lastErr)}
}

func (c *MistralCritic) review(ctx context.Context, chatReq *chatRequest) (*domain.Review, error) {
	resp, err := c.client.chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	var verdict struct {
		Verdicts []struct {
			Claim     string `json:"claim"`
			Supported bool   `json:"supported"`
			Evidence  string `json:"evidence"`
		} `json:"verdicts"`
		VoiceScore    int    `json:"voice_score"`
		VoiceProblems string `json:"voice_problems"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Choices[0].Message.Content)), &verdict); err != nil {
		return nil, fmt.Errorf("failed to parse review: %w", err)
	}
	if verdict.VoiceScore < domain.VoiceScorePoor || verdict.VoiceScore > domain.VoiceScoreGood {
		return nil, fmt.Errorf("voice score out of range: %d", verdict.VoiceScore)
	}

	review := &domain.Review{
		VoiceScore:    verdict.VoiceScore,
		VoiceProblems: strings.TrimSpace(verdict.VoiceProblems),
	}
	for _, v := range verdict.Verdicts {
		review.Verdicts = append(review.Verdicts, domain.ClaimVerdict{
			Claim:     strings.TrimSpace(v.Claim),
			Supported: v.Supported,
			Evidence:  strings.TrimSpace(v.Evidence),
		})
	}
	return review, nil
}

// criticPrompt lays out the story, the draft without its hashtags, and the
// writer's claims.
func criticPrompt(post *domain.Post, content *domain.GeneratedContent) string {
	var b strings.Builder
	b.WriteString("THE STORY:\n")
	if source := post.Article().SourceText(criticSourceWords); source != "" {
		b.WriteString(source + "\n")
	} else {
		b.WriteString("Nothing is known about the story beyond its address.\n")
	}
	fmt.Fprintf(&b, "Published: %s\nLink: %s\n\n", post.Date().Format("January 2, 2006"), post.URI())
	fmt.Fprintf(&b, "THE DRAFT:\n%s\n\n", content.Text())
	b.WriteString("CLAIMS:\n")
	claims := content.Claims()
	if len(claims) == 0 {
		b.WriteString("(none listed)\n")
	}
	for _, claim := range claims {
		fmt.Fprintf(&b, "- %s\n", claim)
	}
	return b.String()
}

// TestConnection verifies the critic's model is still served.
func (c *MistralCritic) TestConnection(ctx context.Context) error {
	card, err := c.client.getModel(ctx, c.model)
	if err != nil {
		return ports.GeneratorError{Model: c.model, Err: err}
	}
	if card.Deprecation != "" {
		if deprecation, err := time.Parse(time.RFC3339, card.Deprecation); err == nil && !deprecation.After(time.Now()) {
			return ports.GeneratorError{Model: c.model, Err: fmt.Errorf("%w: deprecated since %s", ports.ErrModelNotFound, card.Deprecation)}
		}
		slog.Default().Warn("Critic model is scheduled for retirement",
			slog.String("model", c.model),
			slog.String("deprecation", card.Deprecation))
	}
	return nil
}
