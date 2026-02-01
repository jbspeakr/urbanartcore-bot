package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPost_ValidInput(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		dateStr string
	}{
		{
			name:    "valid post",
			uri:     "https://www.urbanartcore.com/test-post/",
			dateStr: "2020-01-15",
		},
		{
			name:    "post with subpath",
			uri:     "https://www.example.com/blog/another-post",
			dateStr: "2015-12-25",
		},
		{
			name:    "edge date",
			uri:     "https://site.com/post",
			dateStr: "2025-06-30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post, err := NewPost(tt.uri, tt.dateStr)

			require.NoError(t, err)
			assert.NotNil(t, post)
			assert.Equal(t, tt.uri, post.URI())
			assert.Equal(t, tt.dateStr, post.DateString())
		})
	}
}

func TestNewPost_InvalidURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{
			name: "empty URI",
			uri:  "",
		},
		{
			name: "malformed URI",
			uri:  "not-a-url",
		},
		{
			name: "URI without scheme",
			uri:  "www.example.com/post",
		},
		{
			name: "URI without host",
			uri:  "https:///post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post, err := NewPost(tt.uri, "2020-01-15")

			assert.Nil(t, post)
			assert.Error(t, err)

			var domainErr DomainError
			assert.True(t, errors.As(err, &domainErr))
			assert.True(t, errors.Is(err, ErrInvalidURI))
		})
	}
}

func TestNewPost_InvalidDate(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
	}{
		{
			name:    "empty date",
			dateStr: "",
		},
		{
			name:    "wrong format",
			dateStr: "01-15-2020",
		},
		{
			name:    "invalid date",
			dateStr: "2020-13-45",
		},
		{
			name:    "year too early",
			dateStr: "1999-01-15",
		},
		{
			name:    "year too late",
			dateStr: "2031-01-15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post, err := NewPost("https://example.com", tt.dateStr)

			assert.Nil(t, post)
			assert.Error(t, err)

			var domainErr DomainError
			assert.True(t, errors.As(err, &domainErr))
			assert.True(t, errors.Is(err, ErrInvalidDate) || errors.Is(err, ErrDateOutOfRange))
		})
	}
}

func TestPost_AnniversaryDateKey(t *testing.T) {
	tests := []struct {
		dateStr  string
		expected string
	}{
		{"2020-01-15", "01-15"},
		{"2020-12-25", "12-25"},
		{"2020-06-30", "06-30"},
	}

	for _, tt := range tests {
		t.Run(tt.dateStr, func(t *testing.T) {
			post, err := NewPost("https://example.com", tt.dateStr)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, post.AnniversaryDateKey())
		})
	}
}

func TestPost_YearsAgo(t *testing.T) {
	post, err := NewPost("https://example.com", "2010-06-15")
	require.NoError(t, err)

	tests := []struct {
		name          string
		referenceDate string
		expected      int
	}{
		{"same year after date", "2010-07-01", 0},
		{"same year before date", "2010-06-14", -1},
		{"10 years later", "2020-07-01", 10},
		{"10 years with anniversary before", "2020-06-14", 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refDate := parseTestDate(t, tt.referenceDate)
			years := post.YearsAgo(*refDate)
			assert.Equal(t, tt.expected, years)
		})
	}
}

func TestPost_IsAnniversary(t *testing.T) {
	post, err := NewPost("https://example.com", "2010-06-15")
	require.NoError(t, err)

	tests := []struct {
		name          string
		checkDate     string
		isAnniversary bool
	}{
		{"exact anniversary", "2020-06-15", true},
		{"same month different day", "2020-06-14", false},
		{"same day different month", "2020-07-15", false},
		{"leap year anniversary", "2020-02-29", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkDate := parseTestDate(t, tt.checkDate)
			is := post.IsAnniversary(*checkDate)
			assert.Equal(t, tt.isAnniversary, is)
		})
	}
}

func TestPost_String(t *testing.T) {
	post, err := NewPost("https://example.com/test", "2020-01-15")
	require.NoError(t, err)

	str := post.String()
	assert.Contains(t, str, "https://example.com/test")
	assert.Contains(t, str, "2020-01-15")
}

func TestPost_Date(t *testing.T) {
	post, err := NewPost("https://example.com", "2020-06-15")
	require.NoError(t, err)

	date := post.Date()
	assert.Equal(t, 2020, date.Year())
	assert.Equal(t, 6, int(date.Month()))
	assert.Equal(t, 15, date.Day())
}

// Helper to parse test dates
func parseTestDate(t *testing.T, dateStr string) *time.Time {
	t.Helper()
	date, err := time.Parse(DateLayout, dateStr)
	require.NoError(t, err)
	return &date
}
