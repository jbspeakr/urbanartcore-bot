// Package ledger keeps the record of published posts in S3-compatible
// object storage (Scaleway Object Storage in production): one JSON document
// per year, rewritten with a conditional PUT so two runs never drop each
// other's records.
package ledger

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

const (
	// DefaultEndpoint is Scaleway Object Storage in Paris.
	DefaultEndpoint = "https://s3.fr-par.scw.cloud"
	// DefaultRegion is the signing region for DefaultEndpoint.
	DefaultRegion = "fr-par"
	// keyPrefix is where the yearly documents live in the bucket.
	keyPrefix = "ledger/"
	// maxDocumentBytes caps a yearly document; a year holds about 730 records.
	maxDocumentBytes = 16 << 20
	// appendAttempts is how often Append retries after a conflicting write.
	appendAttempts = 3
	// requestTimeout bounds one request.
	requestTimeout = 20 * time.Second
)

// Config locates the bucket and holds the credentials that sign requests.
type Config struct {
	Endpoint  string // Base URL; DefaultEndpoint when empty
	Region    string // Signing region; DefaultRegion when empty
	Bucket    string
	AccessKey string
	SecretKey string
}

// ObjectStorage implements ports.Ledger over an S3-compatible bucket.
type ObjectStorage struct {
	cfg  Config
	http *http.Client
	now  func() time.Time
}

// NewObjectStorage creates a ledger in cfg.Bucket. It makes no request.
func NewObjectStorage(cfg Config) (*ObjectStorage, error) {
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}
	if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("invalid ledger endpoint %q: %w", cfg.Endpoint, err)
	}
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("ledger needs a bucket, an access key and a secret key")
	}
	return &ObjectStorage{cfg: cfg, http: &http.Client{Timeout: requestTimeout}, now: time.Now}, nil
}

// document is one year of records as stored in the bucket.
type document struct {
	Records []recordJSON `json:"records"`
}

type recordJSON struct {
	Key         string    `json:"key"`
	Date        string    `json:"date"`
	Program     string    `json:"program"`
	Platform    string    `json:"platform"`
	PostURI     string    `json:"post_uri"`
	RemoteURL   string    `json:"remote_url"`
	Text        string    `json:"text"`
	Fallback    bool      `json:"fallback,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// Records returns every record whose slot date is on or after since, oldest first.
func (s *ObjectStorage) Records(ctx context.Context, since time.Time) ([]domain.PublishRecord, error) {
	from := since.Format(domain.DateLayout)
	var records []domain.PublishRecord
	for year := since.Year(); year <= s.now().Year(); year++ {
		doc, _, err := s.load(ctx, year)
		if err != nil {
			return nil, err
		}
		for _, r := range doc.Records {
			if r.Date >= from {
				records = append(records, r.domain())
			}
		}
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Date < records[j].Date })
	return records, nil
}

// Append adds record to its year's document. The write is conditional on
// the document being unchanged since it was read; on a conflict the
// document is read again and the record added to the newer version. A
// record whose key is already present is not added twice.
func (s *ObjectStorage) Append(ctx context.Context, record domain.PublishRecord) error {
	date, err := time.Parse(domain.DateLayout, record.Date)
	if err != nil {
		return fmt.Errorf("record date %q: %w", record.Date, err)
	}
	for attempt := 1; ; attempt++ {
		doc, etag, err := s.load(ctx, date.Year())
		if err != nil {
			return err
		}
		for _, existing := range doc.Records {
			if existing.Key == record.Key {
				return nil
			}
		}
		doc.Records = append(doc.Records, fromDomain(record))

		err = s.store(ctx, date.Year(), doc, etag)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ports.ErrLedgerConflict) || attempt == appendAttempts {
			return err
		}
		slog.Default().Warn("Ledger changed during write, retrying", slog.Int("attempt", attempt))
	}
}

// load reads one year's document and its ETag. A missing document is an
// empty year with no ETag.
func (s *ObjectStorage) load(ctx context.Context, year int) (*document, string, error) {
	resp, err := s.do(ctx, http.MethodGet, objectKey(year), nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return &document{}, "", nil
	case http.StatusOK:
	default:
		return nil, "", statusError(resp, "read", objectKey(year))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read ledger %s: %w", objectKey(year), err)
	}
	if len(body) > maxDocumentBytes {
		return nil, "", fmt.Errorf("ledger %s is larger than %d bytes", objectKey(year), maxDocumentBytes)
	}
	var doc document
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "", fmt.Errorf("ledger %s is not valid JSON: %w", objectKey(year), err)
	}
	return &doc, resp.Header.Get("ETag"), nil
}

// store writes one year's document: If-Match on the ETag it was read with,
// or If-None-Match: * for a year that did not exist yet.
func (s *ObjectStorage) store(ctx context.Context, year int, doc *document, etag string) error {
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode ledger: %w", err)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if etag != "" {
		headers["If-Match"] = etag
	} else {
		headers["If-None-Match"] = "*"
	}

	resp, err := s.do(ctx, http.MethodPut, objectKey(year), body, headers)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusPreconditionFailed, http.StatusConflict:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return ports.ErrLedgerConflict
	default:
		return statusError(resp, "write", objectKey(year))
	}
}

// do sends one signed path-style request.
func (s *ObjectStorage) do(ctx context.Context, method, key string, body []byte, headers map[string]string) (*http.Response, error) {
	target := s.cfg.Endpoint + "/" + s.cfg.Bucket + "/" + key
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create ledger request: %w", err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	sign(req, body, s.cfg.AccessKey, s.cfg.SecretKey, s.cfg.Region, s.now().UTC())

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ledger %s %s: %w", method, key, err)
	}
	return resp, nil
}

func objectKey(year int) string {
	return fmt.Sprintf("%s%d.json", keyPrefix, year)
}

func statusError(resp *http.Response, op, key string) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("failed to %s ledger %s: HTTP %d: %s", op, key, resp.StatusCode, strings.TrimSpace(string(snippet)))
}

func (r recordJSON) domain() domain.PublishRecord {
	return domain.PublishRecord{
		Key: r.Key, Date: r.Date, Program: r.Program, Platform: r.Platform, PostURI: r.PostURI,
		RemoteURL: r.RemoteURL, Text: r.Text, Fallback: r.Fallback, PublishedAt: r.PublishedAt,
	}
}

func fromDomain(r domain.PublishRecord) recordJSON {
	return recordJSON{
		Key: r.Key, Date: r.Date, Program: r.Program, Platform: r.Platform, PostURI: r.PostURI,
		RemoteURL: r.RemoteURL, Text: r.Text, Fallback: r.Fallback, PublishedAt: r.PublishedAt.UTC(),
	}
}

// sign adds AWS Signature Version 4 headers for the S3 service, signing the
// host, every x-amz-* header and the conditional and content-type headers.
func sign(req *http.Request, body []byte, accessKey, secretKey, region string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	payloadHash := hashHex(body)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-amz-") || lower == "content-type" || lower == "range" ||
			lower == "if-match" || lower == "if-none-match" {
			headers[lower] = strings.TrimSpace(strings.Join(values, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.Query().Encode(),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := day + "/" + region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, hashHex([]byte(canonicalRequest))}, "\n")

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature))
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
