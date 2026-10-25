# Test Data for Mistral Adapter

This directory contains mock API response files used for testing the Mistral adapter.

## Files

### bluesky_response.json
Mock AI-generated content for Bluesky platform.
- **Format**: JSON with `text`, `hashtags`, and `byte_count` fields
- **Purpose**: Simulates successful content generation for Bluesky
- **Constraints**: Text is within the 300-grapheme limit

### mastodon_response.json
Mock AI-generated content for Mastodon platform.
- **Format**: JSON with `text`, `hashtags`, and `character_count` fields
- **Purpose**: Simulates successful content generation for Mastodon
- **Constraints**: Text is within 500-character limit

### invalid_json_response.json
Malformed JSON response for error handling tests.
- **Format**: Invalid JSON (missing closing brace)
- **Purpose**: Tests JSON parsing error handling and retry logic

### empty_text_response.json
Valid JSON but with empty text field.
- **Format**: Valid JSON with empty `text` field
- **Purpose**: Tests content validation and error handling for empty responses

## Usage

These files are loaded by the test suite in `mistral_test.go` to simulate different API response scenarios without making actual API calls. They enable comprehensive testing of:

- Successful content generation
- JSON parsing errors
- Content validation failures
- Platform-specific constraints
- Error handling and retry logic

## Updating Test Data

When updating these files:
1. Ensure JSON is properly formatted (except for `invalid_json_response.json`)
2. Keep content within platform-specific limits
3. Include realistic hashtags and metadata
4. Test changes by running: `go test ./internal/adapters/generator/...`
