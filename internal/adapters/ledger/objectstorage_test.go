package ledger

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// TestSign_AWSExample checks the signer against the GET Object example in
// the AWS Signature Version 4 documentation.
func TestSign_AWSExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=0-9")

	sign(req, nil, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, "+
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, "+
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41",
		req.Header.Get("Authorization"))
}

// fakeBucket is an in-memory S3 bucket that honours If-Match and
// If-None-Match the way Object Storage does.
type fakeBucket struct {
	mu       sync.Mutex
	objects  map[string]string
	versions map[string]int
	puts     int
	// beforePut runs inside a PUT before the precondition check, to
	// simulate another run writing in between
	beforePut func()
}

func newFakeBucket(t *testing.T) (*fakeBucket, *httptest.Server) {
	t.Helper()
	b := &fakeBucket{objects: map[string]string{}, versions: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test-access/"))
		key := strings.TrimPrefix(r.URL.Path, "/uac-ledger/")
		switch r.Method {
		case http.MethodGet:
			b.mu.Lock()
			body, ok := b.objects[key]
			etag := b.etag(key)
			b.mu.Unlock()
			if !ok {
				http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", etag)
			_, _ = io.WriteString(w, body)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			if b.beforePut != nil {
				hook := b.beforePut
				b.beforePut = nil
				hook()
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			_, exists := b.objects[key]
			if match := r.Header.Get("If-Match"); match != "" && (!exists || match != b.etag(key)) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if r.Header.Get("If-None-Match") == "*" && exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			b.objects[key] = string(body)
			b.versions[key]++
			b.puts++
			w.Header().Set("ETag", b.etag(key))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return b, srv
}

func (b *fakeBucket) etag(key string) string {
	return `"v` + strconv.Itoa(b.versions[key]) + `"`
}

func newTestLedger(t *testing.T, endpoint string) *ObjectStorage {
	t.Helper()
	l, err := NewObjectStorage(Config{Endpoint: endpoint, Bucket: "uac-ledger", AccessKey: "test-access", SecretKey: "test-secret"})
	require.NoError(t, err)
	l.now = func() time.Time { return time.Date(2026, 10, 27, 11, 0, 0, 0, time.UTC) }
	return l
}

func record(date, slug, platform string) domain.PublishRecord {
	uri := "https://www.urbanartcore.com/" + slug + "/"
	d, _ := time.Parse(domain.DateLayout, date)
	return domain.PublishRecord{
		Key:         domain.LedgerKey(d, domain.ProgramOnThisDay, uri, platform),
		Date:        date,
		Program:     domain.ProgramOnThisDay,
		Platform:    platform,
		PostURI:     uri,
		RemoteURL:   "https://bsky.app/profile/x/post/" + slug,
		Text:        "text",
		PublishedAt: d.Add(11 * time.Hour),
	}
}

func TestObjectStorage_AppendAndRecords(t *testing.T) {
	bucket, srv := newFakeBucket(t)
	l := newTestLedger(t, srv.URL)
	ctx := context.Background()

	records, err := l.Records(ctx, time.Date(2025, 10, 27, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, records, "missing documents are empty years")

	require.NoError(t, l.Append(ctx, record("2025-12-24", "xmas-wall", domain.PlatformBluesky)))
	require.NoError(t, l.Append(ctx, record("2026-10-27", "banksy-revealed", domain.PlatformBluesky)))
	require.NoError(t, l.Append(ctx, record("2026-10-27", "banksy-revealed", domain.PlatformMastodon)))
	require.NoError(t, l.Append(ctx, record("2026-10-27", "banksy-revealed", domain.PlatformMastodon)), "a repeated key is a no-op")
	assert.Contains(t, bucket.objects, "ledger/2025.json")
	assert.Contains(t, bucket.objects, "ledger/2026.json")
	assert.Equal(t, 3, bucket.puts)

	records, err = l.Records(ctx, time.Date(2025, 10, 27, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, "2025-12-24/on-this-day/xmas-wall/bluesky", records[0].Key)
	assert.Equal(t, "https://www.urbanartcore.com/banksy-revealed/", records[2].PostURI)
	assert.Equal(t, time.Date(2026, 10, 27, 11, 0, 0, 0, time.UTC), records[2].PublishedAt)

	records, err = l.Records(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Len(t, records, 2, "records before since are left out")
}

func TestObjectStorage_AppendRetriesOnConflict(t *testing.T) {
	bucket, srv := newFakeBucket(t)
	l := newTestLedger(t, srv.URL)
	other := newTestLedger(t, srv.URL)
	ctx := context.Background()
	require.NoError(t, l.Append(ctx, record("2026-10-26", "first", domain.PlatformBluesky)))

	// Another run appends between this run's read and its write
	bucket.beforePut = func() {
		require.NoError(t, other.Append(ctx, record("2026-10-27", "other-run", domain.PlatformMastodon)))
	}
	require.NoError(t, l.Append(ctx, record("2026-10-27", "this-run", domain.PlatformBluesky)))

	records, err := l.Records(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	keys := make([]string, 0, len(records))
	for _, r := range records {
		keys = append(keys, r.PostURI)
	}
	assert.ElementsMatch(t, []string{
		"https://www.urbanartcore.com/first/",
		"https://www.urbanartcore.com/other-run/",
		"https://www.urbanartcore.com/this-run/",
	}, keys, "neither run's record is lost")
}

func TestObjectStorage_GivesUpOnPersistentConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	err := newTestLedger(t, srv.URL).Append(context.Background(), record("2026-10-27", "x", domain.PlatformBluesky))
	assert.ErrorIs(t, err, ports.ErrLedgerConflict)
}

func TestObjectStorage_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<Error><Code>AccessDenied</Code></Error>", http.StatusForbidden)
	}))
	defer srv.Close()
	l := newTestLedger(t, srv.URL)

	_, err := l.Records(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.ErrorContains(t, err, "HTTP 403")
	assert.ErrorContains(t, err, "AccessDenied")

	err = l.Append(context.Background(), domain.PublishRecord{Date: "27.10.2026"})
	assert.Error(t, err)
}

func TestObjectStorage_CorruptDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	defer srv.Close()

	_, err := newTestLedger(t, srv.URL).Records(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.ErrorContains(t, err, "not valid JSON")
}

func TestNewObjectStorage_Validation(t *testing.T) {
	_, err := NewObjectStorage(Config{Bucket: "b", AccessKey: "a"})
	assert.Error(t, err)

	l, err := NewObjectStorage(Config{Bucket: "b", AccessKey: "a", SecretKey: "s"})
	require.NoError(t, err)
	assert.Equal(t, DefaultEndpoint, l.cfg.Endpoint)
	assert.Equal(t, DefaultRegion, l.cfg.Region)

	_, err = NewObjectStorage(Config{Endpoint: "not a url", Bucket: "b", AccessKey: "a", SecretKey: "s"})
	assert.Error(t, err)
}
