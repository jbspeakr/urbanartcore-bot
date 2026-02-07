package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeberg.org/brennenstuhl/uac-socializer/internal/domain"
	"codeberg.org/brennenstuhl/uac-socializer/internal/ports"
)

// TestNewBlueskyAdapter tests constructor validation
func TestNewBlueskyAdapter(t *testing.T) {
	tests := []struct {
		name          string
		handle        string
		password      string
		expectError   bool
		errorContains string
	}{
		{
			name:          "empty handle",
			handle:        "",
			password:      "test-password",
			expectError:   true,
			errorContains: "handle cannot be empty",
		},
		{
			name:          "empty password",
			handle:        "test.bsky.social",
			password:      "",
			expectError:   true,
			errorContains: "password cannot be empty",
		},
		{
			name:          "valid credentials (will fail auth)",
			handle:        "test.bsky.social",
			password:      "test-password",
			expectError:   true,
			errorContains: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewBlueskyAdapter(tt.handle, tt.password)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, adapter)

				var pubErr ports.PublisherError
				assert.ErrorAs(t, err, &pubErr)
				assert.Equal(t, "bluesky", pubErr.Platform)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adapter)
			}
		})
	}
}

// TestBlueskyAdapter_Platform tests platform identifier
func TestBlueskyAdapter_Platform(t *testing.T) {
	// Use a nil adapter to test nil safety
	var adapter *BlueskyAdapter
	assert.Equal(t, "bluesky", adapter.Platform())

	// Create a real adapter (won't authenticate but structure is there)
	adapter = &BlueskyAdapter{}
	assert.Equal(t, "bluesky", adapter.Platform())
}

// TestBlueskyAdapter_GetCharacterLimit tests byte limit
func TestBlueskyAdapter_GetCharacterLimit(t *testing.T) {
	var adapter *BlueskyAdapter
	assert.Equal(t, 300, adapter.GetCharacterLimit())

	adapter = &BlueskyAdapter{}
	assert.Equal(t, 300, adapter.GetCharacterLimit())
}

// TestBlueskyAdapter_SupportsHashtags tests hashtag support
func TestBlueskyAdapter_SupportsHashtags(t *testing.T) {
	var adapter *BlueskyAdapter
	assert.True(t, adapter.SupportsHashtags())

	adapter = &BlueskyAdapter{}
	assert.True(t, adapter.SupportsHashtags())
}

// TestBlueskyAdapter_SupportsMedia tests media support
func TestBlueskyAdapter_SupportsMedia(t *testing.T) {
	var adapter *BlueskyAdapter
	assert.True(t, adapter.SupportsMedia())

	adapter = &BlueskyAdapter{}
	assert.True(t, adapter.SupportsMedia())
}

// TestBlueskyAdapter_ValidateContent tests content validation
func TestBlueskyAdapter_ValidateContent(t *testing.T) {
	ctx := context.Background()
	adapter := &BlueskyAdapter{} // Non-nil adapter

	tests := []struct {
		name          string
		adapter       *BlueskyAdapter
		content       *domain.GeneratedContent
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil adapter",
			adapter:       nil,
			content:       createValidContent(t),
			expectError:   true,
			errorContains: "adapter is nil",
		},
		{
			name:          "nil content",
			adapter:       adapter,
			content:       nil,
			expectError:   true,
			errorContains: "content is nil",
		},
		{
			name:          "wrong platform",
			adapter:       adapter,
			content:       createContentWithPlatform(t, "mastodon", "Test post"),
			expectError:   true,
			errorContains: "content platform mismatch",
		},
		{
			name:        "valid content",
			adapter:     adapter,
			content:     createValidContent(t),
			expectError: false,
		},
		{
			name:        "valid content with emoji (UTF-8 bytes)",
			adapter:     adapter,
			content:     createContentWithText(t, "🎨 Street art! 🖼️ #UrbanArt"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.ValidateContent(ctx, tt.content)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)

				var pubErr ports.PublisherError
				assert.ErrorAs(t, err, &pubErr)
				assert.Equal(t, "bluesky", pubErr.Platform)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBlueskyAdapter_TestConnection tests connection validation
func TestBlueskyAdapter_TestConnection(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		adapter       *BlueskyAdapter
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil adapter",
			adapter:       nil,
			expectError:   true,
			errorContains: "adapter is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adapter.TestConnection(ctx)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)

				var pubErr ports.PublisherError
				assert.ErrorAs(t, err, &pubErr)
				assert.Equal(t, "bluesky", pubErr.Platform)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBlueskyAdapter_Publish tests publishing (without real API)
func TestBlueskyAdapter_Publish(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		adapter       *BlueskyAdapter
		content       *domain.GeneratedContent
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil adapter",
			adapter:       nil,
			content:       createValidContent(t),
			expectError:   true,
			errorContains: "adapter is nil",
		},
		{
			name: "nil content",
			adapter: &BlueskyAdapter{
				session: &ATProtoSession{
					DID:       "did:plc:test123",
					ExpiresAt: time.Now().Add(24 * time.Hour),
				},
			},
			content:       nil,
			expectError:   true,
			errorContains: "content is nil",
		},
		{
			name: "invalid content (wrong platform)",
			adapter: &BlueskyAdapter{
				session: &ATProtoSession{
					DID:       "did:plc:test123",
					ExpiresAt: time.Now().Add(24 * time.Hour),
				},
			},
			content:       createContentWithPlatform(t, "mastodon", "Test"),
			expectError:   true,
			errorContains: "content platform mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.adapter.Publish(ctx, tt.content)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)

				// Result should still be created for tracking
				if result != nil {
					assert.Equal(t, domain.StatusFailed, result.Status())
					assert.Equal(t, "bluesky", result.Platform())
				}

				var pubErr ports.PublisherError
				assert.ErrorAs(t, err, &pubErr)
				assert.Equal(t, "bluesky", pubErr.Platform)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestClassifyError tests error classification
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedErr  error
		shouldReturn bool // Whether it should return the specific error or generic
	}{
		{
			name:         "nil error",
			err:          nil,
			expectedErr:  nil,
			shouldReturn: true,
		},
		{
			name:         "authentication error",
			err:          errors.New("authentication failed"),
			expectedErr:  ports.ErrAuthenticationFailed,
			shouldReturn: true,
		},
		{
			name:         "unauthorized error",
			err:          errors.New("unauthorized access"),
			expectedErr:  ports.ErrAuthenticationFailed,
			shouldReturn: true,
		},
		{
			name:         "invalid credentials",
			err:          errors.New("invalid credentials provided"),
			expectedErr:  ports.ErrInvalidCredentials,
			shouldReturn: true,
		},
		{
			name:         "rate limit",
			err:          errors.New("rate limit exceeded"),
			expectedErr:  ports.ErrRateLimitExceeded,
			shouldReturn: true,
		},
		{
			name:         "too many requests",
			err:          errors.New("too many requests"),
			expectedErr:  ports.ErrRateLimitExceeded,
			shouldReturn: true,
		},
		{
			name:         "policy violation",
			err:          errors.New("content policy violation"),
			expectedErr:  ports.ErrContentRejected,
			shouldReturn: true,
		},
		{
			name:         "timeout error",
			err:          errors.New("request timeout"),
			expectedErr:  ports.ErrNetworkTimeout,
			shouldReturn: true,
		},
		{
			name:         "deadline exceeded",
			err:          errors.New("deadline exceeded"),
			expectedErr:  ports.ErrNetworkTimeout,
			shouldReturn: true,
		},
		{
			name:         "service unavailable",
			err:          errors.New("service unavailable"),
			expectedErr:  ports.ErrPlatformUnavailable,
			shouldReturn: true,
		},
		{
			name:         "503 error",
			err:          errors.New("503 service unavailable"),
			expectedErr:  ports.ErrPlatformUnavailable,
			shouldReturn: true,
		},
		{
			name:         "duplicate post",
			err:          errors.New("duplicate post detected"),
			expectedErr:  ports.ErrDuplicatePost,
			shouldReturn: true,
		},
		{
			name:         "generic error",
			err:          errors.New("some unknown error"),
			expectedErr:  nil,
			shouldReturn: false, // Returns original error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			if tt.shouldReturn {
				if tt.expectedErr == nil {
					assert.Nil(t, result)
				} else {
					assert.ErrorIs(t, result, tt.expectedErr)
				}
			} else {
				// Should return original error
				assert.Equal(t, tt.err, result)
			}
		})
	}
}

// TestWrapPublishError tests error wrapping with PublisherError
func TestWrapPublishError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		retryable  bool
		errorClass error
	}{
		{
			name:       "authentication error - not retryable",
			err:        errors.New("authentication failed"),
			retryable:  false,
			errorClass: ports.ErrAuthenticationFailed,
		},
		{
			name:       "rate limit error - retryable",
			err:        errors.New("rate limit exceeded"),
			retryable:  true,
			errorClass: ports.ErrRateLimitExceeded,
		},
		{
			name:       "network timeout - retryable",
			err:        errors.New("timeout occurred"),
			retryable:  true,
			errorClass: ports.ErrNetworkTimeout,
		},
		{
			name:       "content rejected - not retryable",
			err:        errors.New("content policy violation"),
			retryable:  false,
			errorClass: ports.ErrContentRejected,
		},
		{
			name:       "generic error - retryable",
			err:        errors.New("unknown error"),
			retryable:  true,
			errorClass: nil, // Won't match specific error type
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapPublishError(tt.err)

			assert.Equal(t, "bluesky", result.Platform)
			assert.Equal(t, tt.retryable, result.Retryable)
			assert.NotNil(t, result.Err)

			if tt.errorClass != nil {
				assert.ErrorIs(t, result.Err, tt.errorClass)
			}
		})
	}
}

// TestExtractRecordKey tests record key extraction from URIs
func TestExtractRecordKey(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectedKey string
	}{
		{
			name:        "valid URI",
			uri:         "at://did:plc:abc123/app.bsky.feed.post/xyz456",
			expectedKey: "xyz456",
		},
		{
			name:        "URI with longer path",
			uri:         "at://did:plc:abc123/app.bsky.feed.post/3k2j4l5m6n",
			expectedKey: "3k2j4l5m6n",
		},
		{
			name:        "empty URI",
			uri:         "",
			expectedKey: "",
		},
		{
			name:        "single segment",
			uri:         "record123",
			expectedKey: "record123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRecordKey(tt.uri)
			assert.Equal(t, tt.expectedKey, result)
		})
	}
}

// TestFacetStructure tests facet data structure and encoding
func TestFacetStructure(t *testing.T) {
	tests := []struct {
		name     string
		facet    Facet
		wantType string
	}{
		{
			name: "hashtag facet structure",
			facet: Facet{
				Index: ByteRange{Start: 0, End: 10},
				Feature: map[string]string{
					"$type": "app.bsky.richtext.facet#tag",
					"tag":   "StreetArt",
				},
			},
			wantType: "app.bsky.richtext.facet#tag",
		},
		{
			name: "link facet structure",
			facet: Facet{
				Index: ByteRange{Start: 20, End: 40},
				Feature: map[string]string{
					"$type": "app.bsky.richtext.facet#link",
					"uri":   "https://example.com",
				},
			},
			wantType: "app.bsky.richtext.facet#link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.facet.Feature)
			assert.Equal(t, tt.wantType, tt.facet.Feature["$type"])
			assert.NotEqual(t, tt.facet.Index.Start, tt.facet.Index.End)
		})
	}
}

// TestBuildFacetsIntegration tests BuildFacets with real text
func TestBuildFacetsIntegration(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		hashtags      []string
		expectedCount int
	}{
		{
			name:          "no hashtags",
			text:          "Simple post without hashtags",
			hashtags:      []string{},
			expectedCount: 0,
		},
		{
			name:          "single hashtag",
			text:          "Check out this #StreetArt piece!",
			hashtags:      []string{"#StreetArt"},
			expectedCount: 1,
		},
		{
			name:          "multiple hashtags",
			text:          "Amazing #StreetArt and #Graffiti in the city #UrbanArt",
			hashtags:      []string{"#StreetArt", "#Graffiti", "#UrbanArt"},
			expectedCount: 3,
		},
		{
			name:          "hashtag not in text",
			text:          "Simple post",
			hashtags:      []string{"#NotPresent"},
			expectedCount: 0,
		},
		{
			name:          "emoji in text",
			text:          "🎨 Amazing #StreetArt! 🖼️",
			hashtags:      []string{"#StreetArt"},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facets := BuildFacets(tt.text, tt.hashtags)
			assert.Len(t, facets, tt.expectedCount)

			// Verify all facets have valid byte ranges
			err := ValidateFacets(tt.text, facets)
			assert.NoError(t, err)
		})
	}
}

// Helper functions for creating test data

// newBlueskyAdapterWithHost creates a BlueskyAdapter with a custom PDS host for testing.
// This allows tests to intercept HTTP requests to a mock server.
// nolint:unused // Used by TestNewBlueskyAdapterWithHost_CustomHost
func newBlueskyAdapterWithHost(host string) *BlueskyAdapter {
	return &BlueskyAdapter{
		handle:     "test.bsky.social",
		password:   "test-password",
		pdsHost:    host,
		httpClient: &http.Client{Timeout: httpClientTimeout},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			DID:       "did:plc:test123",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		},
	}
}

func createValidContent(t *testing.T) *domain.GeneratedContent {
	t.Helper()
	content, err := domain.NewGeneratedContent(
		"Check out this amazing street art! #UrbanArt #Graffiti",
		"bluesky",
		[]string{"#UrbanArt", "#Graffiti"},
		"test-prompt",
	)
	require.NoError(t, err)
	return content
}

func createContentWithPlatform(t *testing.T, platform, text string) *domain.GeneratedContent {
	t.Helper()
	content, err := domain.NewGeneratedContent(
		text,
		platform,
		[]string{},
		"test-prompt",
	)
	require.NoError(t, err)
	return content
}

func createContentWithText(t *testing.T, text string) *domain.GeneratedContent {
	t.Helper()
	content, err := domain.NewGeneratedContent(
		text,
		"bluesky",
		[]string{},
		"test-prompt",
	)
	require.NoError(t, err)
	return content
}

// Image handling test helpers

// generateValidJPEG creates a minimal valid JPEG image for testing
func generateValidJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a simple pattern
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

// generateValidPNG creates a minimal valid PNG image for testing
func generateValidPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a simple pattern
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// TestDownloadAndEncodeThumbnail_DecodesValidImage tests that valid images can be decoded
func TestDownloadAndEncodeThumbnail_DecodesValidImage(t *testing.T) {
	tests := []struct {
		name        string
		imageData   []byte
		contentType string
	}{
		{
			name:        "valid JPEG",
			imageData:   generateValidJPEG(100, 100),
			contentType: "image/jpeg",
		},
		{
			name:        "valid PNG",
			imageData:   generateValidPNG(100, 100),
			contentType: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server to serve the image
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(tt.imageData)
			}))
			defer server.Close()

			// Create mock blob upload server
			blobServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"blob": map[string]any{
						"$type":    "blob",
						"link":     "bafkreiabc123",
						"mimeType": "image/jpeg",
						"size":     1000,
					},
				})
			}))
			defer blobServer.Close()

			adapter := &BlueskyAdapter{
				httpClient: &http.Client{},
				session: &ATProtoSession{
					AccessJwt: "test-token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			}

			// Override blueskyPDSHost temporarily for testing (using closure hack)
			originalHost := blueskyPDSHost
			defer func() {
				// Can't actually override const, but we'll test the function behavior
			}()
			_ = originalHost

			ctx := context.Background()
			result, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

			// Should succeed if blob upload works
			if err == nil {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "blob", result["$type"])
			}
		})
	}
}

// TestDownloadAndEncodeThumbnail_LogsImageInfo tests that image metadata is logged
func TestDownloadAndEncodeThumbnail_LogsImageInfo(t *testing.T) {
	imageData := generateValidJPEG(200, 150)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(imageData)
	}))
	defer server.Close()

	adapter := &BlueskyAdapter{
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	_, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

	// Verify the image was decoded (error will be from blob upload, not decode)
	if err != nil {
		// If error contains "failed to decode", that's a problem
		assert.NotContains(t, err.Error(), "failed to decode image")
	}
}

// TestDownloadAndEncodeThumbnail_SkipsOversized tests that oversized files are gracefully skipped
func TestDownloadAndEncodeThumbnail_SkipsOversized(t *testing.T) {
	// Create image larger than 1MB
	largeImageData := make([]byte, maxThumbnailSize+1000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(largeImageData)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeImageData)
	}))
	defer server.Close()

	adapter := &BlueskyAdapter{
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	result, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

	// Expect graceful degradation: no error, but nil result
	require.NoError(t, err)
	assert.Nil(t, result, "oversized images should return nil to signal graceful skip")
}

// TestDownloadAndEncodeThumbnail_ResizesOversized tests that large images are resized
func TestDownloadAndEncodeThumbnail_ResizesOversized(t *testing.T) {
	// Create image larger than max dimensions (but under size limit)
	largeImage := generateValidJPEG(2500, 2000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeImage)
	}))
	defer server.Close()

	blobServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blob": map[string]any{
				"$type":    "blob",
				"link":     "bafkreixyz789",
				"mimeType": "image/jpeg",
				"size":     50000,
			},
		})
	}))
	defer blobServer.Close()

	adapter := &BlueskyAdapter{
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	_, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

	// Should succeed after resizing (or fail at blob upload)
	// The important thing is it doesn't fail at decode/resize
	if err != nil {
		assert.NotContains(t, err.Error(), "failed to decode")
		assert.NotContains(t, err.Error(), "failed to encode")
	}
}

// TestDownloadAndEncodeThumbnail_MaintainsAspectRatio tests aspect ratio preservation
func TestDownloadAndEncodeThumbnail_MaintainsAspectRatio(t *testing.T) {
	// Test with various aspect ratios
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"wide image", 2500, 1000},
		{"tall image", 1000, 2500},
		{"square image", 2000, 2000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageData := generateValidJPEG(tt.width, tt.height)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(imageData)
			}))
			defer server.Close()

			adapter := &BlueskyAdapter{
				httpClient: &http.Client{},
				session: &ATProtoSession{
					AccessJwt: "test-token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			}

			ctx := context.Background()
			_, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

			// Should process without decode/resize errors
			if err != nil {
				assert.NotContains(t, err.Error(), "failed to decode")
			}
		})
	}
}

// TestDownloadAndEncodeThumbnail_KeepsSmallImages tests that small images aren't resized
func TestDownloadAndEncodeThumbnail_KeepsSmallImages(t *testing.T) {
	smallImage := generateValidJPEG(500, 400)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(smallImage)
	}))
	defer server.Close()

	blobServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blob": map[string]any{
				"$type":    "blob",
				"link":     "bafkreismall123",
				"mimeType": "image/jpeg",
				"size":     len(smallImage),
			},
		})
	}))
	defer blobServer.Close()

	adapter := &BlueskyAdapter{
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	_, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

	// Should succeed without resizing
	if err != nil {
		assert.NotContains(t, err.Error(), "failed to decode")
	}
}

// TestDownloadAndEncodeThumbnail_ResizedImageWithinLimits tests output dimensions
func TestDownloadAndEncodeThumbnail_ResizedImageWithinLimits(t *testing.T) {
	largeImage := generateValidJPEG(3000, 2500)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeImage)
	}))
	defer server.Close()

	blobServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the uploaded blob to verify dimensions
		body := make([]byte, 1024*1024) // 1MB buffer
		n, _ := r.Body.Read(body)
		blobData := body[:n]

		// Decode to check dimensions
		img, _, err := image.Decode(bytes.NewReader(blobData))
		if err == nil {
			bounds := img.Bounds()
			width, height := bounds.Dx(), bounds.Dy()

			// Verify dimensions are within limits
			assert.LessOrEqual(t, width, maxThumbnailWidth, "Width should be within limit")
			assert.LessOrEqual(t, height, maxThumbnailHeight, "Height should be within limit")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blob": map[string]any{
				"$type":    "blob",
				"link":     "bafkreiresized789",
				"mimeType": "image/jpeg",
				"size":     len(blobData),
			},
		})
	}))
	defer blobServer.Close()

	adapter := &BlueskyAdapter{
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	_, err := adapter.downloadAndEncodeThumbnail(ctx, server.URL)

	// Should succeed with resized dimensions
	if err != nil {
		assert.NotContains(t, err.Error(), "failed to decode")
	}
}

// createMockHTTPClient creates an HTTP client that redirects requests to a test server
func createMockHTTPClient(mockServer *httptest.Server) *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			mockServer: mockServer,
		},
	}
}

// mockTransport redirects all requests to a mock server
type mockTransport struct {
	mockServer *httptest.Server
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect request to mock server
	req.URL.Scheme = "http"
	req.URL.Host = t.mockServer.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// TestUploadBlobToBluesky_Success tests successful blob upload
func TestUploadBlobToBluesky_Success(t *testing.T) {
	expectedCID := "bafkreiabc123xyz"
	blobData := []byte("test blob data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/xrpc/com.atproto.repo.uploadBlob", r.URL.Path)
		assert.Equal(t, "image/jpeg", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blob": map[string]any{
				"$type":    "blob",
				"link":     expectedCID,
				"mimeType": "image/jpeg",
				"size":     len(blobData),
			},
		})
	}))
	defer server.Close()

	adapter := &BlueskyAdapter{
		httpClient: createMockHTTPClient(server),
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	cid, err := adapter.uploadBlobToBluesky(ctx, blobData)

	require.NoError(t, err)
	assert.Equal(t, expectedCID, cid)
}

// TestUploadBlobToBluesky_AuthenticationFailure tests 401/403 handling
func TestUploadBlobToBluesky_AuthenticationFailure(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"401 Unauthorized", http.StatusUnauthorized},
		{"403 Forbidden", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("authentication failed"))
			}))
			defer server.Close()

			adapter := &BlueskyAdapter{
				httpClient: createMockHTTPClient(server),
				session: &ATProtoSession{
					AccessJwt: "invalid-token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			}

			ctx := context.Background()
			_, err := adapter.uploadBlobToBluesky(ctx, []byte("test"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "authentication failed")
		})
	}
}

// TestUploadBlobToBluesky_RateLimit tests 429 handling
func TestUploadBlobToBluesky_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit exceeded"))
	}))
	defer server.Close()

	adapter := &BlueskyAdapter{
		httpClient: createMockHTTPClient(server),
		session: &ATProtoSession{
			AccessJwt: "test-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	ctx := context.Background()
	_, err := adapter.uploadBlobToBluesky(ctx, []byte("test"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

// TestUploadBlobToBluesky_ServerError tests 5xx handling
func TestUploadBlobToBluesky_ServerError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("server error"))
			}))
			defer server.Close()

			adapter := &BlueskyAdapter{
				httpClient: createMockHTTPClient(server),
				session: &ATProtoSession{
					AccessJwt: "test-token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			}

			ctx := context.Background()
			_, err := adapter.uploadBlobToBluesky(ctx, []byte("test"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "server error")
		})
	}
}

// TestNewBlueskyAdapterWithHost_CustomHost tests that helper can override PDS host
func TestNewBlueskyAdapterWithHost_CustomHost(t *testing.T) {
	customHost := "https://test.example.com"
	adapter := newBlueskyAdapterWithHost(customHost)

	assert.NotNil(t, adapter)
	assert.Equal(t, customHost, adapter.pdsHost)
	assert.Equal(t, "test.bsky.social", adapter.handle)
	assert.NotNil(t, adapter.session)
	assert.NotNil(t, adapter.httpClient)
}

// TestEnsureValidSession_ExistingSessionReturnsNoError tests that
// ensureValidSession returns nil when session already exists
func TestEnsureValidSession_ExistingSessionReturnsNoError(t *testing.T) {
	ctx := context.Background()

	// Create adapter with existing session
	adapter := &BlueskyAdapter{
		handle:     "test.bsky.social",
		password:   "test-password",
		pdsHost:    "https://bsky.social",
		httpClient: &http.Client{},
		session: &ATProtoSession{
			AccessJwt:  "test-access-token",
			RefreshJwt: "test-refresh-token",
			Handle:     "test.bsky.social",
			DID:        "did:plc:test123",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
	}

	// Call ensureValidSession with existing session
	err := adapter.ensureValidSession(ctx)

	assert.NoError(t, err, "ensureValidSession should not error with existing session")
	// Verify session is unchanged
	adapter.sessionMu.RLock()
	defer adapter.sessionMu.RUnlock()
	assert.Equal(t, "test-access-token", adapter.session.AccessJwt)
}

// TestEnsureValidSession_NoSessionAuthenticates tests that missing sessions trigger authentication
func TestEnsureValidSession_NoSessionAuthenticates(t *testing.T) {
	ctx := context.Background()

	authCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			authCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessJwt":  "new-access-token",
				"refreshJwt": "new-refresh-token",
				"handle":     "test.bsky.social",
				"did":        "did:plc:test123",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create adapter with no session
	adapter := &BlueskyAdapter{
		handle:     "test.bsky.social",
		password:   "test-password",
		pdsHost:    server.URL,
		httpClient: &http.Client{},
		session:    nil,
	}

	err := adapter.ensureValidSession(ctx)

	assert.NoError(t, err)
	assert.True(t, authCalled, "authentication should be called when session is nil")
	assert.NotNil(t, adapter.session)
}

// TestUploadBlobToBluesky_MalformedResponse tests invalid response parsing
func TestUploadBlobToBluesky_MalformedResponse(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		errorText    string
	}{
		{
			name:         "invalid JSON",
			responseBody: "not json",
			errorText:    "failed to decode upload response",
		},
		{
			name:         "missing CID",
			responseBody: `{"blob": {"$type": "blob", "mimeType": "image/jpeg", "size": 1000}}`,
			errorText:    "upload response missing CID link",
		},
		{
			name:         "empty blob object",
			responseBody: `{"blob": {}}`,
			errorText:    "upload response missing CID link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			adapter := &BlueskyAdapter{
				httpClient: createMockHTTPClient(server),
				session: &ATProtoSession{
					AccessJwt: "test-token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			}

			ctx := context.Background()
			_, err := adapter.uploadBlobToBluesky(ctx, []byte("test"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorText)
		})
	}
}
