package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

const (
	// maxMediaBytes is the largest image the bot uploads; Mastodon accepts
	// 16 MB, the site's covers are well below 1 MB.
	maxMediaBytes = 10 << 20
	// mediaPollAttempts bounds the wait for Mastodon to finish processing an upload.
	mediaPollAttempts = 10
)

// mediaPollInterval is the pause between two checks on an upload; tests shorten it.
var mediaPollInterval = time.Second

// mediaAttachment is the part of a Mastodon MediaAttachment the bot reads.
// URL stays null while the server is still processing the file.
type mediaAttachment struct {
	ID  string  `json:"id"`
	URL *string `json:"url"`
}

// uploadImages uploads each image with its alt text and returns the media
// ids of those that are ready. An image that fails is logged and left out:
// the status goes out without it rather than not at all.
func (a *MastodonAdapter) uploadImages(ctx context.Context, images []domain.Image) []string {
	ids := make([]string, 0, len(images))
	for _, img := range images {
		id, err := a.uploadImage(ctx, img)
		if err != nil {
			slog.Default().Warn("Image upload failed, posting without it",
				slog.String("platform", domain.PlatformMastodon),
				slog.String("image", img.URL),
				slog.String("error", err.Error()))
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// uploadImage downloads img, uploads it to /api/v2/media with its alt text
// and waits until the server has processed it.
func (a *MastodonAdapter) uploadImage(ctx context.Context, img domain.Image) (string, error) {
	data, contentType, err := a.downloadImage(ctx, img.URL)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, imageFilename(img.URL)))
	header.Set("Content-Type", contentType)
	part, err := form.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := form.WriteField("description", img.Alt); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+"/api/v2/media", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.accessToken)
	req.Header.Set("Content-Type", form.FormDataContentType())

	media, status, err := a.doMedia(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return "", fmt.Errorf("media upload returned status %d", status)
	}
	if media.URL != nil {
		return media.ID, nil
	}
	return media.ID, a.waitForMedia(ctx, media.ID)
}

// waitForMedia polls a media attachment until the server has processed it.
func (a *MastodonAdapter) waitForMedia(ctx context.Context, id string) error {
	for attempt := 0; attempt < mediaPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(mediaPollInterval):
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.server+"/api/v1/media/"+id, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+a.accessToken)
		media, status, err := a.doMedia(req)
		if err != nil {
			return err
		}
		if status == http.StatusOK && media.URL != nil {
			return nil
		}
		if status != http.StatusOK && status != http.StatusPartialContent {
			return fmt.Errorf("media status returned %d", status)
		}
	}
	return fmt.Errorf("media %s still processing after %d checks", id, mediaPollAttempts)
}

// doMedia sends a media request and decodes the attachment from a 2xx response.
func (a *MastodonAdapter) doMedia(req *http.Request) (*mediaAttachment, int, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var media mediaAttachment
	if err := json.Unmarshal(raw, &media); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to decode media attachment: %w", err)
	}
	if media.ID == "" {
		return nil, resp.StatusCode, errors.New("media attachment without id")
	}
	return &media, resp.StatusCode, nil
}

// downloadImage fetches an image and returns its bytes and content type.
func (a *MastodonAdapter) downloadImage(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", domain.UserAgent)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image download returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image: %w", err)
	}
	if len(data) > maxMediaBytes {
		return nil, "", fmt.Errorf("image is larger than %d bytes", maxMediaBytes)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("not an image: %s", contentType)
	}
	return data, contentType, nil
}

// imageFilename is the last path segment of url, or "image".
func imageFilename(url string) string {
	name := path.Base(strings.SplitN(url, "?", 2)[0])
	if name == "" || name == "." || name == "/" {
		return "image"
	}
	return name
}
