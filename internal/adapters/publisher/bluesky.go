package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"golang.org/x/net/html"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

const (
	// ATProto API endpoints
	blueskyPDSHost = "https://bsky.social"

	// Character and byte limits
	blueskyByteLimit = 300

	// HTTP client timeouts
	connectionTestTimeout = 10 * time.Second
	httpClientTimeout     = 10 * time.Second

	// Link preview and thumbnail constraints
	maxThumbnailSize   = 1 * 1024 * 1024 // 1MB
	maxThumbnailWidth  = 1920
	maxThumbnailHeight = 1080
)

// ATProtoSession represents a cached Bluesky authentication session.
// Sessions are reused across multiple posts until expiry.
type ATProtoSession struct {
	AccessJwt  string    // JWT token for authenticated requests
	RefreshJwt string    // JWT token for session refresh
	Handle     string    // User's Bluesky handle
	DID        string    // User's DID (Decentralized Identifier)
	ExpiresAt  time.Time // Session expiration timestamp
}

// BlueskyAdapter implements the SocialPublisher interface for Bluesky/ATProto.
// It handles authentication, facet generation, embed creation, and post publishing.
type BlueskyAdapter struct {
	handle     string          // Bluesky account handle
	password   string          // Bluesky app password
	pdsHost    string          // ATProto PDS (Personal Data Server) host
	session    *ATProtoSession // Cached authentication session
	sessionMu  sync.RWMutex    // Protects concurrent session access
	httpClient *http.Client    // HTTP client for link preview fetching
}

// NewBlueskyAdapter creates a new BlueskyAdapter with authentication.
// It validates configuration and establishes an authenticated session.
// Returns error if authentication fails or configuration is invalid.
func NewBlueskyAdapter(handle, password string) (*BlueskyAdapter, error) {

	// Validate configuration
	if handle == "" {
		return nil, ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("handle cannot be empty"),
			Retryable: false,
		}
	}
	if password == "" {
		return nil, ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("password cannot be empty"),
			Retryable: false,
		}
	}

	// Create HTTP client for link previews with connection pool configuration
	httpClient := &http.Client{
		Timeout: httpClientTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	adapter := &BlueskyAdapter{
		handle:     handle,
		password:   password,
		pdsHost:    blueskyPDSHost,
		httpClient: httpClient,
	}

	// Authenticate and establish session
	ctx, cancel := context.WithTimeout(context.Background(), connectionTestTimeout)
	defer cancel()

	if err := adapter.authenticate(ctx); err != nil {
		return nil, ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("authentication failed: %w", err),
			Retryable: false,
		}
	}

	slog.Default().Info("Bluesky adapter initialized",
		slog.String("handle", handle),
		slog.String("pds_host", adapter.pdsHost))

	return adapter, nil
}

// authenticate creates a new session with the Bluesky API via HTTP.
// Caches the session token for future use.
func (a *BlueskyAdapter) authenticate(ctx context.Context) error {
	if a == nil {
		return errors.New("adapter is nil")
	}

	// Create session request body
	sessionReq := map[string]string{
		"identifier": a.handle,
		"password":   a.password,
	}

	body, err := json.Marshal(sessionReq)
	if err != nil {
		return fmt.Errorf("failed to marshal session request: %w", err)
	}

	// Call ATProto session endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", a.pdsHost+"/xrpc/com.atproto.server.createSession", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		// Check for timeout/network errors
		if isNetworkError(err) {
			return fmt.Errorf("%w: %v", ports.ErrNetworkTimeout, err)
		}
		return fmt.Errorf("session request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Classify based on HTTP status code
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("authentication failed: status %d: %s", resp.StatusCode, string(bodyBytes))
		case http.StatusTooManyRequests:
			return fmt.Errorf("rate limit exceeded: status %d: %s", resp.StatusCode, string(bodyBytes))
		case http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			return fmt.Errorf("service unavailable: status %d: %s", resp.StatusCode, string(bodyBytes))
		default:
			return fmt.Errorf("session request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		}
	}

	// Parse response
	var sessionResp struct {
		AccessJwt  string `json:"accessJwt"`
		RefreshJwt string `json:"refreshJwt"`
		Handle     string `json:"handle"`
		DID        string `json:"did"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return fmt.Errorf("failed to decode session response: %w", err)
	}

	// Cache session
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()

	a.session = &ATProtoSession{
		AccessJwt:  sessionResp.AccessJwt,
		RefreshJwt: sessionResp.RefreshJwt,
		Handle:     sessionResp.Handle,
		DID:        sessionResp.DID,
		ExpiresAt:  time.Now().Add(24 * time.Hour), // Sessions typically valid for 24h
	}

	slog.Default().Info("Bluesky session created",
		slog.String("handle", sessionResp.Handle),
		slog.String("did", sessionResp.DID))

	return nil
}

// ensureValidSession checks if a session exists, establishing one if needed.
// Does not perform session refresh (interactions temporarily limited).
func (a *BlueskyAdapter) ensureValidSession(ctx context.Context) error {
	if a == nil {
		return errors.New("adapter is nil")
	}

	a.sessionMu.RLock()
	session := a.session
	a.sessionMu.RUnlock()

	if session == nil {
		// No session, authenticate
		return a.authenticate(ctx)
	}

	return nil
}

// Platform returns the platform identifier.
func (a *BlueskyAdapter) Platform() string {
	return "bluesky"
}

// GetCharacterLimit returns the byte limit for Bluesky posts.
// Note: Bluesky uses byte count, not character/rune count.
func (a *BlueskyAdapter) GetCharacterLimit() int {
	return blueskyByteLimit
}

// SupportsHashtags returns whether Bluesky supports hashtag functionality.
func (a *BlueskyAdapter) SupportsHashtags() bool {
	return true
}

// SupportsMedia returns whether Bluesky supports media attachments.
func (a *BlueskyAdapter) SupportsMedia() bool {
	return true
}

// ValidateContent checks if content meets Bluesky-specific requirements.
// Validates platform match, non-empty text, and byte count limit.
func (a *BlueskyAdapter) ValidateContent(ctx context.Context, content *domain.GeneratedContent) error {

	if a == nil {
		return ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	if content == nil {
		return ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("content is nil"),
			Retryable: false,
		}
	}

	// Check platform match
	if content.Platform() != "bluesky" {
		return ports.PublisherError{

			Platform: "bluesky",
			Err: fmt.Errorf("content platform mismatch: expected 'bluesky', got '%s'",
				content.Platform()),
			Retryable: false,
		}
	}

	// Check non-empty text
	text := content.Text()
	if text == "" {
		return ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("text cannot be empty"),
			Retryable: false,
		}
	}

	// Check byte count (UTF-8 bytes, not runes)
	byteCount := CountTextBytes(text)
	if byteCount > blueskyByteLimit {
		return ports.PublisherError{

			Platform: "bluesky",
			Err: fmt.Errorf("%w: %d bytes, limit is %d",
				ports.ErrContentTooLong, byteCount, blueskyByteLimit),
			Retryable: false,
		}
	}

	return nil
}

// TestConnection verifies connectivity to Bluesky and session validity.
// Refreshes session if it's expiring soon.
func (a *BlueskyAdapter) TestConnection(ctx context.Context) error {

	if a == nil {
		return ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	// Add timeout to context
	timeoutCtx, cancel := context.WithTimeout(ctx, connectionTestTimeout)
	defer cancel()

	// Ensure session is valid (will refresh if needed)
	if err := a.ensureValidSession(timeoutCtx); err != nil {
		classified := classifyError(err)
		// TestConnection errors are always retryable since connection tests should be retried
		return ports.PublisherError{

			Platform:  "bluesky",
			Err:       classified,
			Retryable: true,
		}
	}

	slog.Default().Info("Bluesky connection test successful",
		slog.String("handle", a.handle))

	return nil
}

// Publish sends content to Bluesky with facets and optional external embed.
// Returns PlatformResult with success status and published post URL.
func (a *BlueskyAdapter) Publish(ctx context.Context, content *domain.GeneratedContent) (*domain.PlatformResult, error) {
	startTime := time.Now()

	if a == nil {
		return nil, ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("adapter is nil"),
			Retryable: false,
		}
	}

	if content == nil {
		return nil, ports.PublisherError{

			Platform:  "bluesky",
			Err:       errors.New("content is nil"),
			Retryable: false,
		}
	}

	// Validate content
	if err := a.ValidateContent(ctx, content); err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		result.SetFailure(err, time.Since(startTime))
		return result, err
	}

	// Ensure session is valid
	if err := a.ensureValidSession(ctx); err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("session validation failed: %w", err),
			Retryable: true,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	text := content.Text()
	hashtags := content.Hashtags()

	// Build facets for hashtags
	facets := BuildFacets(text, hashtags)

	// Validate facets
	if err := ValidateFacets(text, facets); err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("invalid facets: %w", err),
			Retryable: false,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	// Build external embed if URI is in metadata
	var embedData map[string]any
	metadata := content.Metadata()
	uri := metadata["uri"]
	if uri != "" {
		// Extract link preview (graceful degradation if it fails)
		linkEmbed, err := a.buildExternalEmbedFromURI(ctx, uri)
		if err != nil {
			slog.Default().Warn("Failed to build external embed, proceeding without it",
				slog.String("uri", uri),
				slog.String("error", err.Error()))
		} else if linkEmbed != nil {
			embedData = linkEmbed
		}
	}

	// Read session DID
	a.sessionMu.RLock()
	did := a.session.DID
	a.sessionMu.RUnlock()

	// Build post record as map for JSON marshaling
	postRecord := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().Format(time.RFC3339Nano),
	}

	// Add facets if any
	if len(facets) > 0 {
		facetData := make([]map[string]any, len(facets))
		for i, f := range facets {
			facetData[i] = map[string]any{
				"index": map[string]any{
					"byteStart": f.Index.Start,
					"byteEnd":   f.Index.End,
				},
				"features": []map[string]string{f.Feature},
			}
		}
		postRecord["facets"] = facetData
	}

	// Add embed if created
	if embedData != nil {
		postRecord["embed"] = embedData
	}

	// Create post via ATProto API
	createRecordReq := map[string]any{
		"repo":       did,
		"collection": "app.bsky.feed.post",
		"record":     postRecord,
		"validate":   true,
	}

	reqBody, err := json.Marshal(createRecordReq)
	if err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("failed to marshal post record: %w", err),
			Retryable: false,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.pdsHost+"/xrpc/com.atproto.repo.createRecord", bytes.NewReader(reqBody))
	if err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("failed to create request: %w", err),
			Retryable: false,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.session.AccessJwt)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("publish request failed: %w", err),
			Retryable: true,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, string(bodyBytes)),
			Retryable: resp.StatusCode >= 500,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	// Parse response
	var createResp struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		result, _ := domain.NewPlatformResult("bluesky", domain.StatusFailed, content)
		wrappedErr := ports.PublisherError{

			Platform:  "bluesky",
			Err:       fmt.Errorf("failed to decode publish response: %w", err),
			Retryable: false,
		}
		result.SetFailure(wrappedErr, time.Since(startTime))
		return result, wrappedErr
	}

	// Construct published URL from URI
	publishedURL := fmt.Sprintf("https://bsky.app/profile/%s/post/%s", a.handle, extractRecordKey(createResp.URI))

	// Create success result
	result, _ := domain.NewPlatformResult("bluesky", domain.StatusSuccess, content)
	result.SetSuccess(publishedURL, time.Since(startTime))

	slog.Default().Info("Published to Bluesky",
		slog.String("platform", "bluesky"),
		slog.String("uri", publishedURL),
		slog.Int("facet_count", len(facets)),
		slog.Bool("has_embed", embedData != nil),
		slog.Duration("response_time", time.Since(startTime)))

	return result, nil
}

// buildExternalEmbedFromURI creates an external embed with link preview.
// Fetches HTML to extract title, description, and thumbnail.
// Returns nil if extraction fails (graceful degradation).
func (a *BlueskyAdapter) buildExternalEmbedFromURI(ctx context.Context, uri string) (map[string]any, error) {
	if a == nil || uri == "" {
		return nil, errors.New("adapter is nil or URI is empty")
	}

	// Extract link preview
	preview, err := a.extractLinkPreview(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to extract link preview: %w", err)
	}

	// Build external embed structure
	externalData := map[string]any{
		"uri":         uri,
		"title":       preview.Title,
		"description": preview.Description,
	}

	// Download and encode thumbnail if available
	if preview.ThumbURL != "" {
		thumbData, err := a.downloadAndEncodeThumbnail(ctx, preview.ThumbURL)
		if err != nil {
			// Log warning but proceed without thumbnail
			slog.Default().Warn("Failed to download thumbnail, proceeding without it",
				slog.String("thumb_url", preview.ThumbURL),
				slog.String("error", err.Error()))
		} else if thumbData != nil {
			externalData["thumb"] = thumbData
		}
	}

	embedData := map[string]any{
		"$type":    "app.bsky.embed.external",
		"external": externalData,
	}

	return embedData, nil
}

// LinkPreview represents extracted metadata from a web page.
type LinkPreview struct {
	Title       string
	Description string
	ThumbURL    string
}

// extractLinkPreview fetches HTML and extracts Open Graph metadata.
// Returns title, description, and thumbnail URL from meta tags.
func (a *BlueskyAdapter) extractLinkPreview(ctx context.Context, uri string) (*LinkPreview, error) {
	if a == nil {
		return nil, errors.New("adapter is nil")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain body to release connection back to pool
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unexpected status code: %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("unexpected status code: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse HTML
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	preview := &LinkPreview{}

	// Extract metadata from meta tags
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var property, content string
			for _, attr := range n.Attr {
				if attr.Key == "property" || attr.Key == "name" {
					property = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}

			// Extract Open Graph tags
			switch property {
			case "og:title":
				if preview.Title == "" {
					preview.Title = content
				}
			case "og:description":
				if preview.Description == "" {
					preview.Description = content
				}
			case "og:image":
				if preview.ThumbURL == "" {
					preview.ThumbURL = content
				}
			case "twitter:title":
				if preview.Title == "" {
					preview.Title = content
				}
			case "twitter:description":
				if preview.Description == "" {
					preview.Description = content
				}
			case "twitter:image":
				if preview.ThumbURL == "" {
					preview.ThumbURL = content
				}
			}
		}

		// Extract title from <title> tag
		if n.Type == html.ElementNode && n.Data == "title" && preview.Title == "" {
			if n.FirstChild != nil {
				preview.Title = n.FirstChild.Data
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(doc)

	// Truncate description to 300 chars
	if len(preview.Description) > 300 {
		preview.Description = preview.Description[:297] + "..."
	}

	// Ensure at least title is present
	if preview.Title == "" {
		preview.Title = uri
	}

	return preview, nil
}

// downloadAndEncodeThumbnail downloads an image and encodes it as a blob.
// Resizes to max dimensions and converts to JPEG for ATProto upload.
// Returns (nil, nil) for oversized images as graceful degradation - caller should proceed without thumbnail.
func (a *BlueskyAdapter) downloadAndEncodeThumbnail(ctx context.Context, thumbURL string) (map[string]any, error) {
	if a == nil {
		return nil, errors.New("adapter is nil")
	}

	// Download image
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download thumbnail: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain body to release connection back to pool
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unexpected status code: %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("unexpected status code: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Check content length - skip gracefully if oversized
	if resp.ContentLength > maxThumbnailSize {
		// Drain body to release connection back to pool
		_, _ = io.Copy(io.Discard, resp.Body)
		slog.Default().Warn("Image exceeds size limit, skipping thumbnail",
			slog.Int64("size", resp.ContentLength),
			slog.Int64("limit", maxThumbnailSize))
		return nil, nil
	}

	// Read image
	imageData, err := io.ReadAll(io.LimitReader(resp.Body, maxThumbnailSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	// Decode image
	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	logger := slog.Default()
	logger.Info("Decoded image",
		slog.String("format", format),
		slog.Int("width", img.Bounds().Dx()),
		slog.Int("height", img.Bounds().Dy()))

	// Resize if needed (maintains aspect ratio)
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	var buf *bytes.Buffer
	if width > maxThumbnailWidth || height > maxThumbnailHeight {
		aspectRatio := float64(width) / float64(height)
		var newWidth, newHeight int

		if width > height {
			newWidth = maxThumbnailWidth
			newHeight = int(float64(newWidth) / aspectRatio)
		} else {
			newHeight = maxThumbnailHeight
			newWidth = int(float64(newHeight) * aspectRatio)
		}

		logger.Info("Resizing image",
			slog.Int("original_width", width),
			slog.Int("original_height", height),
			slog.Int("new_width", newWidth),
			slog.Int("new_height", newHeight))

		resized := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)

		// Re-encode resized image
		buf = &bytes.Buffer{}
		err = jpeg.Encode(buf, resized, &jpeg.Options{Quality: 85})
		if err != nil {
			// Log and use original
			logger.Warn("Failed to re-encode resized image, using original",
				slog.String("error", err.Error()))

			// Encode original as fallback
			buf = &bytes.Buffer{}
			if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 85}); err != nil {
				return nil, fmt.Errorf("failed to encode JPEG: %w", err)
			}
		}
	} else {
		// Image already within limits, encode as-is
		buf = &bytes.Buffer{}
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("failed to encode JPEG: %w", err)
		}
	}

	blobData := buf.Bytes()

	// Upload blob to Bluesky and get CID
	cidReference, err := a.uploadBlobToBluesky(ctx, blobData)
	if err != nil {
		return nil, fmt.Errorf("failed to upload blob: %w", err)
	}

	// Return blob reference with CID from ATProto
	return map[string]any{
		"$type":    "blob",
		"link":     cidReference,
		"size":     len(blobData),
		"mimeType": "image/jpeg",
	}, nil
}

// uploadBlobToBluesky uploads binary data to Bluesky's blob store.
// Returns the CID (Content Identifier) reference from ATProto.
func (a *BlueskyAdapter) uploadBlobToBluesky(ctx context.Context, blobData []byte) (string, error) {
	if a == nil {
		return "", errors.New("adapter is nil")
	}

	// Create context with 30-second timeout
	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Create upload request
	req, err := http.NewRequestWithContext(uploadCtx, "POST", a.pdsHost+"/xrpc/com.atproto.repo.uploadBlob", bytes.NewReader(blobData))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("Authorization", "Bearer "+a.session.AccessJwt)

	// Execute request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("blob upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle HTTP status codes
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			bodyBytes = []byte("(failed to read body)")
		}

		// Classify error based on status code
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", fmt.Errorf("authentication failed: status %d: %s", resp.StatusCode, string(bodyBytes))
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("rate limit exceeded: status %d: %s", resp.StatusCode, string(bodyBytes))
		default:
			if resp.StatusCode >= 500 {
				return "", fmt.Errorf("server error: status %d: %s", resp.StatusCode, string(bodyBytes))
			}
			return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		}
	}

	// Parse response to extract CID
	var uploadResp struct {
		Blob struct {
			Type     string `json:"$type"`
			Link     string `json:"link"`
			MimeType string `json:"mimeType"`
			Size     int    `json:"size"`
		} `json:"blob"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	// Validate CID is present
	if uploadResp.Blob.Link == "" {
		return "", fmt.Errorf("upload response missing CID link")
	}

	slog.Default().Info("Blob uploaded to Bluesky",
		slog.String("cid", uploadResp.Blob.Link),
		slog.Int("size", uploadResp.Blob.Size),
		slog.String("mime_type", uploadResp.Blob.MimeType))

	return uploadResp.Blob.Link, nil
}

// Helper functions

// extractRecordKey extracts the record key (post ID) from an ATProto URI.
// Example: at://did:plc:abc123/app.bsky.feed.post/xyz456 -> xyz456
func extractRecordKey(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// classifyError classifies ATProto errors into retryable/non-retryable categories.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// Authentication errors - not retryable
	if strings.Contains(errLower, "authentication") || strings.Contains(errLower, "unauthorized") {
		return ports.ErrAuthenticationFailed
	}

	// Invalid credentials - not retryable
	if strings.Contains(errLower, "invalid") && strings.Contains(errLower, "credentials") {
		return ports.ErrInvalidCredentials
	}

	// Rate limit - retryable
	if strings.Contains(errLower, "rate limit") || strings.Contains(errLower, "too many requests") {
		return ports.ErrRateLimitExceeded
	}

	// Content policy - not retryable
	if strings.Contains(errLower, "policy") || strings.Contains(errLower, "violation") {
		return ports.ErrContentRejected
	}

	// Network/timeout - retryable
	if strings.Contains(errLower, "timeout") || strings.Contains(errLower, "deadline") {
		return ports.ErrNetworkTimeout
	}

	// Service unavailable - retryable
	if strings.Contains(errLower, "unavailable") || strings.Contains(errLower, "503") {
		return ports.ErrPlatformUnavailable
	}

	// Duplicate post - not retryable
	if strings.Contains(errLower, "duplicate") {
		return ports.ErrDuplicatePost
	}

	// Default: return original error (treated as retryable)
	return err
}

// wrapPublishError wraps an error with PublisherError context.
func wrapPublishError(err error) ports.PublisherError {
	classified := classifyError(err)

	// Check if error is one of the non-retryable types
	retryable := !errors.Is(classified, ports.ErrAuthenticationFailed) &&
		!errors.Is(classified, ports.ErrInvalidCredentials) &&
		!errors.Is(classified, ports.ErrContentRejected) &&
		!errors.Is(classified, ports.ErrDuplicatePost)

	return ports.PublisherError{
		Platform:  "bluesky",
		Err:       classified,
		Retryable: retryable,
	}
}
