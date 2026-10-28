package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultMistralEndpoint is the public Mistral API base URL.
const defaultMistralEndpoint = "https://api.mistral.ai"

// Chat message roles.
const (
	roleSystem = "system"
	roleUser   = "user"
)

// mistralClient is a minimal REST client for the two Mistral endpoints the
// bot needs: chat completions with structured output, and model lookup.
// It makes exactly one HTTP request per call; retries are the adapter's job.
type mistralClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func newMistralClient(apiKey, endpoint string, timeout time.Duration) *mistralClient {
	if endpoint == "" {
		endpoint = defaultMistralEndpoint
	}
	return &mistralClient{
		apiKey:     apiKey,
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat asks the API to constrain the output. With type
// "json_schema" the API validates the response against the schema.
type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	TopP           float64         `json:"top_p"`
	MaxTokens      int             `json:"max_tokens"`
	SafePrompt     bool            `json:"safe_prompt"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// contentPart is one part of a multimodal message: text, or an image the
// API fetches from its URL.
type contentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type partsMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

// partsRequest is a chat request whose messages may carry images.
type partsRequest struct {
	Model       string         `json:"model"`
	Messages    []partsMessage `json:"messages"`
	Temperature float64        `json:"temperature"`
	TopP        float64        `json:"top_p"`
	MaxTokens   int            `json:"max_tokens"`
}

// modelCard is the part of GET /v1/models/{id} the bot reads.
type modelCard struct {
	ID          string `json:"id"`
	Deprecation string `json:"deprecation,omitempty"`
	// Capabilities says what the model accepts; Vision means image input
	Capabilities struct {
		Vision bool `json:"vision"`
	} `json:"capabilities"`
}

// chat sends one chat completion request.
func (c *mistralClient) chat(ctx context.Context, req *chatRequest) (*chatResponse, error) {
	var resp chatResponse
	if err := c.do(ctx, http.MethodPost, "/v1/chat/completions", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// chatWithParts sends one chat completion request with multimodal messages.
func (c *mistralClient) chatWithParts(ctx context.Context, req *partsRequest) (*chatResponse, error) {
	var resp chatResponse
	if err := c.do(ctx, http.MethodPost, "/v1/chat/completions", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// getModel looks up a model by id. A model the API no longer serves
// returns an error containing "model not found".
func (c *mistralClient) getModel(ctx context.Context, model string) (*modelCard, error) {
	var card modelCard
	err := c.do(ctx, http.MethodGet, "/v1/models/"+url.PathEscape(model), nil, &card)
	if err != nil {
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("model not found: %s", model)
		}
		return nil, err
	}
	return &card, nil
}

// do performs one request and decodes a JSON response into out.
// Non-2xx responses become an HTTPStatusError whose message carries the
// status code, so classifyError can sort them.
func (c *mistralClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
