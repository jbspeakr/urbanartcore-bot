package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDateGroup_ValidInput(t *testing.T) {
	// Create test posts
	post1, err := NewPost("https://example.com/post1", "2020-01-15")
	require.NoError(t, err)

	post2, err := NewPost("https://example.com/post2", "2021-01-15")
	require.NoError(t, err)

	posts := []*Post{post1, post2}

	dg, err := NewDateGroup("01-15", posts)

	require.NoError(t, err)
	assert.NotNil(t, dg)
	assert.Equal(t, "01-15", dg.DateKey())
	assert.Equal(t, 2, dg.Count())
	assert.False(t, dg.IsEmpty())

	// Test that posts are copied
	retrievedPosts := dg.Posts()
	assert.Len(t, retrievedPosts, 2)
	assert.Equal(t, post1.URI(), retrievedPosts[0].URI())
	assert.Equal(t, post2.URI(), retrievedPosts[1].URI())
}

func TestNewDateGroup_InvalidDateKey(t *testing.T) {
	post, err := NewPost("https://example.com/post", "2020-01-15")
	require.NoError(t, err)

	tests := []struct {
		name    string
		dateKey string
	}{
		{"empty date key", ""},
		{"wrong format - no dash", "0115"},
		{"wrong format - too long", "01-155"},
		{"wrong format - too short", "1-15"},
		{"invalid month", "13-15"},
		{"invalid day", "01-32"},
		{"zero month", "00-15"},
		{"zero day", "01-00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dg, err := NewDateGroup(tt.dateKey, []*Post{post})

			assert.Nil(t, dg)
			assert.Error(t, err)

			var domainErr DomainError
			assert.True(t, errors.As(err, &domainErr))
			assert.True(t, errors.Is(err, ErrInvalidDateKey))
		})
	}
}

func TestNewDateGroup_EmptyPosts(t *testing.T) {
	dg, err := NewDateGroup("01-15", []*Post{})

	assert.Nil(t, dg)
	assert.Error(t, err)

	var domainErr DomainError
	assert.True(t, errors.As(err, &domainErr))
	assert.True(t, errors.Is(err, ErrEmptyDateGroup))
}

func TestNewDateGroup_NilPost(t *testing.T) {
	post, err := NewPost("https://example.com/post", "2020-01-15")
	require.NoError(t, err)

	posts := []*Post{post, nil}

	dg, err := NewDateGroup("01-15", posts)

	assert.Nil(t, dg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "post at index 1 is nil")
}

func TestNewDateGroup_DateKeyMismatch(t *testing.T) {
	post, err := NewPost("https://example.com/post", "2020-01-15")
	require.NoError(t, err)

	// Try to create group with wrong date key
	dg, err := NewDateGroup("01-16", []*Post{post})

	assert.Nil(t, dg)
	assert.Error(t, err)

	var domainErr DomainError
	assert.True(t, errors.As(err, &domainErr))
	assert.True(t, errors.Is(err, ErrDateKeyMismatch))
}

func TestNewDateGroup_DuplicatePosts(t *testing.T) {
	post1, err := NewPost("https://example.com/post", "2020-01-15")
	require.NoError(t, err)

	post2, err := NewPost("https://example.com/post", "2021-01-15") // Same URI
	require.NoError(t, err)

	posts := []*Post{post1, post2}

	dg, err := NewDateGroup("01-15", posts)

	assert.Nil(t, dg)
	assert.Error(t, err)

	var domainErr DomainError
	assert.True(t, errors.As(err, &domainErr))
	assert.True(t, errors.Is(err, ErrDuplicatePost))
}

func TestParseDateKey_ValidKeys(t *testing.T) {
	tests := []struct {
		dateKey string
	}{
		{"01-01"},
		{"12-31"},
		{"06-15"},
		{"02-29"}, // Leap day
	}

	for _, tt := range tests {
		t.Run(tt.dateKey, func(t *testing.T) {
			dg, err := ParseDateKey(tt.dateKey)

			require.NoError(t, err)
			assert.Equal(t, tt.dateKey, dg.DateKey())
			assert.Equal(t, 0, dg.Count())
			assert.True(t, dg.IsEmpty())
		})
	}
}

func TestParseDateKey_InvalidKeys(t *testing.T) {
	tests := []struct {
		name    string
		dateKey string
	}{
		{"empty", ""},
		{"wrong format", "1-1"},
		{"invalid month", "13-01"},
		{"invalid day", "01-32"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dg, err := ParseDateKey(tt.dateKey)

			assert.Nil(t, dg)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidDateKey))
		})
	}
}

func TestFromTime(t *testing.T) {
	testTime := time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC)

	dg, err := FromTime(testTime)

	require.NoError(t, err)
	assert.Equal(t, "06-15", dg.DateKey())
	assert.True(t, dg.IsEmpty())
}

func TestDateGroup_GetPost(t *testing.T) {
	post1, err := NewPost("https://example.com/post1", "2020-01-15")
	require.NoError(t, err)

	post2, err := NewPost("https://example.com/post2", "2021-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post1, post2})
	require.NoError(t, err)

	// Test valid indices
	assert.Equal(t, post1, dg.GetPost(0))
	assert.Equal(t, post2, dg.GetPost(1))

	// Test invalid indices
	assert.Nil(t, dg.GetPost(-1))
	assert.Nil(t, dg.GetPost(2))
}

func TestDateGroup_AddPost(t *testing.T) {
	post1, err := NewPost("https://example.com/post1", "2020-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post1})
	require.NoError(t, err)

	// Add a valid post
	post2, err := NewPost("https://example.com/post2", "2021-01-15")
	require.NoError(t, err)

	newDg, err := dg.AddPost(post2)
	require.NoError(t, err)

	// Original group unchanged
	assert.Equal(t, 1, dg.Count())

	// New group has both posts
	assert.Equal(t, 2, newDg.Count())
	assert.Equal(t, post1, newDg.GetPost(0))
	assert.Equal(t, post2, newDg.GetPost(1))
}

func TestDateGroup_AddPost_Invalid(t *testing.T) {
	post1, err := NewPost("https://example.com/post1", "2020-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post1})
	require.NoError(t, err)

	// Try to add nil post
	newDg, err := dg.AddPost(nil)
	assert.Nil(t, newDg)
	assert.Error(t, err)

	// Try to add post with wrong date
	wrongPost, err := NewPost("https://example.com/wrong", "2020-01-16")
	require.NoError(t, err)

	newDg, err = dg.AddPost(wrongPost)
	assert.Nil(t, newDg)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrDateKeyMismatch))

	// Try to add duplicate post
	dupPost, err := NewPost("https://example.com/post1", "2021-01-15")
	require.NoError(t, err)

	newDg, err = dg.AddPost(dupPost)
	assert.Nil(t, newDg)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrDuplicatePost))
}

func TestDateGroup_MonthDay(t *testing.T) {
	dg, err := ParseDateKey("06-15")
	require.NoError(t, err)

	month, day, err := dg.MonthDay()
	require.NoError(t, err)

	assert.Equal(t, 6, month)
	assert.Equal(t, 15, day)
}

func TestDateGroup_ToTime(t *testing.T) {
	dg, err := ParseDateKey("06-15")
	require.NoError(t, err)

	testTime, err := dg.ToTime(2023)
	require.NoError(t, err)

	expected := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	assert.True(t, expected.Equal(testTime))
}

func TestDateGroup_ToTime_InvalidDate(t *testing.T) {
	dg, err := ParseDateKey("02-29") // Leap day
	require.NoError(t, err)

	// Try non-leap year
	_, err = dg.ToTime(2023)
	assert.Error(t, err)
}

func TestDateGroup_IsToday(t *testing.T) {
	today := time.Now().Format("01-02")
	todayGroup, err := ParseDateKey(today)
	require.NoError(t, err)

	notTodayGroup, err := ParseDateKey("12-25")
	require.NoError(t, err)

	assert.True(t, todayGroup.IsToday())

	// This might be flaky if test runs on Christmas, but very unlikely
	if today != "12-25" {
		assert.False(t, notTodayGroup.IsToday())
	}
}

func TestDateGroup_IsAnniversaryOf(t *testing.T) {
	dg, err := ParseDateKey("06-15")
	require.NoError(t, err)

	testTime := time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC)
	wrongTime := time.Date(2023, 6, 16, 10, 30, 0, 0, time.UTC)

	assert.True(t, dg.IsAnniversaryOf(testTime))
	assert.False(t, dg.IsAnniversaryOf(wrongTime))
}

func TestDateGroup_YearsRange(t *testing.T) {
	// Test empty group
	emptyGroup, err := ParseDateKey("01-15")
	require.NoError(t, err)

	min, max := emptyGroup.YearsRange()
	assert.Equal(t, 0, min)
	assert.Equal(t, 0, max)

	// Test group with posts
	post1, err := NewPost("https://example.com/post1", "2018-01-15")
	require.NoError(t, err)

	post2, err := NewPost("https://example.com/post2", "2023-01-15")
	require.NoError(t, err)

	post3, err := NewPost("https://example.com/post3", "2020-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post1, post2, post3})
	require.NoError(t, err)

	min, max = dg.YearsRange()
	assert.Equal(t, 2018, min)
	assert.Equal(t, 2023, max)
}

func TestDateGroup_String(t *testing.T) {
	post, err := NewPost("https://example.com/post", "2020-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post})
	require.NoError(t, err)

	result := dg.String()
	assert.Contains(t, result, "01-15")
	assert.Contains(t, result, "1")
}

func TestDateKeyPattern(t *testing.T) {
	// Test the regex pattern directly
	validKeys := []string{
		"01-01", "01-31", "02-28", "02-29",
		"03-31", "04-30", "05-31", "06-30",
		"07-31", "08-31", "09-30", "10-31",
		"11-30", "12-31",
	}

	invalidKeys := []string{
		"", "1-1", "01-1", "1-01",
		"13-01", "01-32", "00-01", "01-00",
		"1-15", "01-5", "1-5",
	}

	for _, key := range validKeys {
		assert.True(t, DateKeyPattern.MatchString(key), "Expected %s to be valid", key)
	}

	for _, key := range invalidKeys {
		assert.False(t, DateKeyPattern.MatchString(key), "Expected %s to be invalid", key)
	}
}

func TestDateGroup_PostsImmutability(t *testing.T) {
	post1, err := NewPost("https://example.com/post1", "2020-01-15")
	require.NoError(t, err)

	post2, err := NewPost("https://example.com/post2", "2021-01-15")
	require.NoError(t, err)

	dg, err := NewDateGroup("01-15", []*Post{post1, post2})
	require.NoError(t, err)

	// Get posts and modify the slice
	posts := dg.Posts()
	posts[0] = nil // This shouldn't affect the original

	// Original should be unchanged
	assert.NotNil(t, dg.GetPost(0))
	assert.Equal(t, 2, dg.Count())
}
