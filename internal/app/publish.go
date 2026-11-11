package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
	"github.com/jbspeakr/urbanartcore-bot/internal/ports"
)

// ApplicationError represents errors from the application layer.
type ApplicationError struct {
	Err error // Underlying error
}

func (e ApplicationError) Error() string {
	return e.Err.Error()
}

func (e ApplicationError) Unwrap() error {
	return e.Err
}

// Common application errors
var (
	ErrInvalidDateKey   = errors.New("invalid date key format")
	ErrNoPlatforms      = errors.New("no platforms available for publishing")
	ErrContextCancelled = errors.New("operation was cancelled")
	ErrCriticalFailure  = errors.New("critical failure stopped operation")
	ErrPartialFailure   = errors.New("some platforms failed to publish")
)

// PublishRequest contains all parameters needed for a publishing operation.
type PublishRequest struct {
	DateKey   string   // MM-DD format date key
	Platforms []string // Target platforms (bluesky, mastodon, all)
	DryRun    bool     // Whether to perform a dry run (generate but don't publish)
	Program   string   // The day's program; empty follows domain.Rhythm
}

// PublishResponse contains the aggregated results of a publishing operation.
type PublishResponse struct {
	Result    *domain.PublishResult               // Aggregated publishing results
	Generated map[string]*domain.GeneratedContent // Generated content per platform
	Error     error                               // Any critical errors that occurred
	RestDay   bool                                // No post for the date: nothing was generated or published
	Published bool                                // Every platform already has this slot's post: nothing was done
}

// PublishService orchestrates the complete publishing workflow for anniversary posts.
// It coordinates between the repository, content generator, and social publishers
// to provide a unified publishing experience with proper error handling and logging.
type PublishService struct {
	repository ports.PostRepository
	generator  ports.ContentGenerator
	publishers map[string]ports.SocialPublisher
	reader     ports.PostReader     // optional; without it the model only sees what the catalogue holds
	critic     ports.Critic         // optional; without it drafts are published unchecked
	ledger     ports.Ledger         // optional; without it reruns post again and posts repeat freely
	describer  ports.ImageDescriber // optional; without it alt text comes from the site's cover alt or the title
	archive    ports.Archive        // needed by the feature programs only
	now        func() time.Time
}

// NewPublishService creates a new PublishService with the provided dependencies.
func NewPublishService(
	repository ports.PostRepository,
	generator ports.ContentGenerator,
	publishers map[string]ports.SocialPublisher,
) *PublishService {
	// Nil safety: initialize empty map if nil
	if publishers == nil {
		publishers = make(map[string]ports.SocialPublisher)
	}

	return &PublishService{
		repository: repository,
		generator:  generator,
		publishers: publishers,
		now:        time.Now,
	}
}

// WithPostReader lets the service read each post's full article, so the
// generated text can promote what the post is about.
func (ps *PublishService) WithPostReader(reader ports.PostReader) *PublishService {
	ps.reader = reader
	return ps
}

// WithCritic has every draft checked against its post before it is
// published. A rejected draft is rewritten with the critic's findings up to
// MaxRewrites times, then replaced by the fallback template.
func (ps *PublishService) WithCritic(critic ports.Critic) *PublishService {
	ps.critic = critic
	return ps
}

// WithLedger records every published post and consults the record before
// each run: a platform that already has today's post is skipped, and posts
// featured within domain.RepeatWindow are passed over when the day offers
// another.
func (ps *PublishService) WithLedger(ledger ports.Ledger) *PublishService {
	ps.ledger = ledger
	return ps
}

// WithImageDescriber has a model write the alt text of each post's cover
// photograph.
func (ps *PublishService) WithImageDescriber(describer ports.ImageDescriber) *PublishService {
	ps.describer = describer
	return ps
}

// WithArchive gives the feature programs the whole archive: collections,
// artists and every post.
func (ps *PublishService) WithArchive(archive ports.Archive) *PublishService {
	ps.archive = archive
	return ps
}

// MaxRewrites is how often a draft the critic rejects is written again
// before the fallback template takes its place.
const MaxRewrites = 2

// PublishAnniversaryPosts executes the complete publishing workflow.
// It handles all 6 orchestration steps with proper error handling and context management.
func (ps *PublishService) PublishAnniversaryPosts(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	// Step 1: Input Validation
	if err := ps.validateRequest(req); err != nil {
		return nil, ApplicationError{Err: err}
	}

	// Nil safety checks for service dependencies
	// These checks happen after validation to allow testing of validation logic
	// In production, these should never be nil due to container initialization
	if ps.repository == nil {
		return nil, ApplicationError{Err: errors.New("repository is required but is nil")}
	}
	if ps.generator == nil {
		return nil, ApplicationError{Err: errors.New("generator is required but is nil")}
	}

	slog.Default().Info("Starting anniversary post publishing",
		slog.String("dateKey", req.DateKey),
		slog.Any("platforms", req.Platforms),
		slog.Bool("dryRun", req.DryRun))

	// Step 2: Data Retrieval
	slot := ps.slotDate(req.DateKey)
	history, err := ps.history(ctx, slot)
	if err != nil {
		return nil, ApplicationError{Err: err}
	}
	planned, err := NewPlanner(ps.repository, ps.archive, ps.ledger).Slot(ctx, slot, history, req.Program)
	if err != nil {
		return nil, ApplicationError{Err: fmt.Errorf("failed to plan the day: %w", err)}
	}
	if planned.IsRestDay() {
		// Some days have nothing worth posting: no anniversary post, or only
		// posts featured within a year. Post nothing and exit cleanly.
		slog.Default().Info("Rest day: nothing to publish",
			slog.String("dateKey", req.DateKey), slog.Any("reasons", planned.Reasons))
		return &PublishResponse{RestDay: true}, nil
	}
	slog.Default().Info("Planned the day",
		slog.String("program", planned.Program),
		slog.String("subject", planned.Subject()),
		slog.Float64("score", planned.Score),
		slog.Any("reasons", planned.Reasons))
	if planned.Feature != nil {
		return ps.publishFeature(ctx, req, slot, history, *planned.Feature)
	}
	post, program := planned.Post, planned.Program

	// Nil safety check: ensure post is not nil
	if post == nil {
		return nil, ApplicationError{Err: errors.New("repository returned nil post")}
	}

	slog.Default().Info("Retrieved post for anniversary date",
		slog.String("dateKey", req.DateKey),
		slog.String("uri", post.URI()))

	platforms, done, err := ps.targetPlatforms(req, slot, history)
	if done != nil || err != nil {
		return done, err
	}

	post = ps.readPost(ctx, post)
	return ps.publishPost(ctx, req, slot, platforms, postJob{
		post:    post,
		program: program,
		images:  ps.images(ctx, post),
	})
}

// postJob is what a run posts: the post the writer works from, the photos
// that go with it, the program the ledger records it under, and any context
// the prompt needs beyond the post.
type postJob struct {
	post    *domain.Post
	program string
	images  []domain.Image
	context map[string]string
}

// shareApproved gives a platform left with the fallback template the text
// another platform's draft passed review with: the critic checked the same
// claims against the same post, and a real sentence beats the template.
func shareApproved(contents map[string]*domain.GeneratedContent) {
	var approved *domain.GeneratedContent
	for _, platform := range []string{domain.PlatformBluesky, domain.PlatformMastodon} {
		if content := contents[platform]; content != nil && !content.IsFallback() {
			approved = content
			break
		}
	}
	if approved == nil {
		return
	}
	for platform, content := range contents {
		if !content.IsFallback() {
			continue
		}
		shared, err := domain.Retarget(approved, platform)
		if err != nil {
			slog.Default().Warn("Could not reuse the approved text, keeping the fallback template",
				slog.String("platform", platform), slog.String("error", err.Error()))
			continue
		}
		contents[platform] = shared.WithImages(content.Images())
		slog.Default().Info("Using the text approved for another platform instead of the fallback template",
			slog.String("platform", platform), slog.String("from", approved.Platform()),
			slog.String("text", shared.Text()))
	}
}

// targetPlatforms resolves the request's platforms and drops those that
// already have the slot's post. A response instead of platforms means there
// is nothing left to do.
func (ps *PublishService) targetPlatforms(req *PublishRequest, slot time.Time, history []domain.PublishRecord) ([]string, *PublishResponse, error) {
	platforms, err := ps.resolvePlatforms(req.Platforms)
	if err != nil {
		return nil, nil, ApplicationError{Err: err}
	}
	if len(platforms) == 0 {
		return nil, nil, ApplicationError{Err: ErrNoPlatforms}
	}

	// A rerun of the same slot posts nothing where the post already went out
	platforms = ps.unpublished(platforms, slot, history)
	if len(platforms) == 0 {
		slog.Default().Info("Already published: every platform has this slot's post",
			slog.String("slot", slot.Format(domain.DateLayout)))
		return nil, &PublishResponse{Published: true}, nil
	}

	slog.Default().Info("Resolved target platforms", slog.Any("platforms", platforms))
	return platforms, nil, nil
}

// publishPost writes the job's post for each platform, publishes it and
// records each success in the ledger.
func (ps *PublishService) publishPost(ctx context.Context, req *PublishRequest, slot time.Time, platforms []string, job postJob) (*PublishResponse, error) {
	post, images := job.post, job.images

	// Initialize result tracking
	publishResult, err := domain.NewPublishResult(post, req.DryRun)
	if err != nil {
		return nil, ApplicationError{Err: fmt.Errorf("failed to create publish result: %w", err)}
	}

	generatedContent := make(map[string]*domain.GeneratedContent)

	// Step 4: Content Generation Loop
	for _, platform := range platforms {
		select {
		case <-ctx.Done():
			slog.Default().Warn("Content generation cancelled by context")
			return nil, ApplicationError{Err: ErrContextCancelled}
		default:
		}

		slog.Default().Info("Generating content for platform", slog.String("platform", platform))

		content, err := ps.generateContentForPlatform(ctx, post, platform, req.DateKey, req.DryRun, job.context)
		if err != nil {
			// Platform error - continue with other platforms
			slog.Default().Error("Content generation failed for platform",
				slog.String("platform", platform),
				slog.String("error", err.Error()))

			// Create failed platform result
			platformResult, _ := domain.NewPlatformResult(platform, domain.StatusFailed, nil)
			platformResult.SetFailure(fmt.Errorf("content generation failed: %w", err), 0)
			if err := publishResult.AddPlatformResult(platformResult); err != nil {
				slog.Default().Error("Failed to add platform result", slog.String("error", err.Error()))
			}
			continue
		}

		generatedContent[platform] = content.WithImages(images)
		slog.Default().Info("Content generated successfully",
			slog.String("platform", platform),
			slog.Int("characters", content.CharacterCount()),
			slog.Int("images", len(images)))
	}

	shareApproved(generatedContent)

	// Check if we have any content to publish
	if len(generatedContent) == 0 {
		return nil, ApplicationError{Err: errors.New("failed to generate content for any platform")}
	}

	// Step 5: Publishing Loop
	for platform, content := range generatedContent {
		select {
		case <-ctx.Done():
			slog.Default().Warn("Publishing cancelled by context")
			return nil, ApplicationError{Err: ErrContextCancelled}
		default:
		}

		slog.Default().Info("Publishing to platform", slog.String("platform", platform))

		platformResult, err := ps.publishToPlatform(ctx, platform, content, req.DryRun)
		if err != nil {
			// Platform error - continue with other platforms
			slog.Default().Error("Publishing failed for platform",
				slog.String("platform", platform),
				slog.String("error", err.Error()))

			if platformResult == nil {
				platformResult, _ = domain.NewPlatformResult(platform, domain.StatusFailed, content)
				platformResult.SetFailure(err, 0)
			}
		}

		if err := publishResult.AddPlatformResult(platformResult); err != nil {
			slog.Default().Error("Failed to add platform result", slog.String("error", err.Error()))
		}

		if !req.DryRun && platformResult != nil && platformResult.IsSuccess() {
			ps.record(ctx, slot, job.program, post.URI(), platformResult)
		}
	}

	// Step 6: Result Aggregation
	publishResult.Complete()

	slog.Default().Info("Publishing operation completed",
		slog.Int("successCount", publishResult.SuccessCount()),
		slog.Int("failureCount", publishResult.FailureCount()),
		slog.Duration("duration", publishResult.Duration()))

	response := &PublishResponse{
		Result:    publishResult,
		Generated: generatedContent,
	}

	// Determine if we had partial failures
	if publishResult.HasFailures() {
		if publishResult.SuccessCount() == 0 {
			response.Error = ErrCriticalFailure
		} else {
			response.Error = ErrPartialFailure
		}
	}

	return response, nil
}

// slotDate is the calendar day being filled: dateKey (MM-DD) in the current
// year. February 29 outside a leap year becomes March 1.
func (ps *PublishService) slotDate(dateKey string) time.Time {
	now := ps.now().UTC()
	slot, err := time.Parse(domain.DateLayout, fmt.Sprintf("%d-%s", now.Year(), dateKey))
	if err != nil {
		var month, day int
		_, _ = fmt.Sscanf(dateKey, "%d-%d", &month, &day)
		return time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}
	return slot
}

// history reads the ledger records of the repeat window before slot. A
// configured ledger that cannot be read stops the run, since it can no
// longer tell whether today's post already went out.
func (ps *PublishService) history(ctx context.Context, slot time.Time) ([]domain.PublishRecord, error) {
	if ps.ledger == nil {
		return nil, nil
	}
	records, err := ps.ledger.Records(ctx, slot.Add(-domain.RepeatWindow))
	if err != nil {
		return nil, fmt.Errorf("failed to read the ledger: %w", err)
	}
	return records, nil
}

// unpublished drops the platforms the ledger already holds a post for on slot.
func (ps *PublishService) unpublished(platforms []string, slot time.Time, history []domain.PublishRecord) []string {
	day := slot.Format(domain.DateLayout)
	done := make(map[string]string)
	for _, r := range history {
		if r.Date == day {
			done[r.Platform] = r.RemoteURL
		}
	}
	left := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		if remote, ok := done[platform]; ok {
			slog.Default().Info("Skipping platform: today's post already went out",
				slog.String("platform", platform), slog.String("remote_url", remote))
			continue
		}
		left = append(left, platform)
	}
	return left
}

// record appends a published post to the ledger. A failed write is logged,
// not returned: the post is out, and failing the run would invite a retry
// that posts it twice.
func (ps *PublishService) record(ctx context.Context, slot time.Time, program, uri string, result *domain.PlatformResult) {
	if ps.ledger == nil {
		return
	}
	content := result.Content()
	rec := domain.PublishRecord{
		Key:         domain.LedgerKey(slot, program, uri, result.Platform()),
		Date:        slot.Format(domain.DateLayout),
		Program:     program,
		Platform:    result.Platform(),
		PostURI:     uri,
		RemoteURL:   result.PublishedURL(),
		Text:        content.RenderedText(),
		Fallback:    content.IsFallback(),
		PublishedAt: result.PublishedAt(),
	}
	if rec.PublishedAt.IsZero() {
		rec.PublishedAt = ps.now()
	}
	if err := ps.ledger.Append(ctx, rec); err != nil {
		slog.Default().Error("Published, but the ledger write failed: a rerun today may post again",
			slog.String("key", rec.Key),
			slog.String("remote_url", rec.RemoteURL),
			slog.String("error", err.Error()))
		return
	}
	slog.Default().Info("Recorded in the ledger", slog.String("key", rec.Key))
}

// validateRequest validates the publishing request parameters.
func (ps *PublishService) validateRequest(req *PublishRequest) error {
	// Nil safety: handle nil receiver gracefully
	if ps == nil {
		return errors.New("publish service cannot be nil")
	}

	if req == nil {
		return errors.New("request cannot be nil")
	}

	// Validate date key format (MM-DD)
	dateKey := strings.TrimSpace(req.DateKey)
	if dateKey == "" {
		return fmt.Errorf("%w: date key cannot be empty", ErrInvalidDateKey)
	}

	dateKeyPattern := regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
	if !dateKeyPattern.MatchString(dateKey) {
		return fmt.Errorf("%w: expected MM-DD format, got %q", ErrInvalidDateKey, dateKey)
	}

	// Validate platforms
	if len(req.Platforms) == 0 {
		return errors.New("at least one platform must be specified")
	}

	for _, platform := range req.Platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if !domain.IsValidPlatform(platform) {
			return fmt.Errorf("invalid platform: %s", platform)
		}
	}

	switch req.Program {
	case "", domain.ProgramOnThisDay, domain.ProgramPlace, domain.ProgramCollection, domain.ProgramArtist, domain.ProgramBackThen:
	default:
		return fmt.Errorf("unknown program %q", req.Program)
	}

	return nil
}

// resolvePlatforms converts platform names to normalized platform list.
func (ps *PublishService) resolvePlatforms(platforms []string) ([]string, error) {
	// Nil safety check
	if ps == nil {
		return nil, errors.New("publish service is nil")
	}
	if ps.publishers == nil {
		return nil, errors.New("publishers map is nil")
	}

	resolved := make([]string, 0)

	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))

		if platform == domain.PlatformAll {
			// Expand "all" to all available publishers
			for publisherPlatform := range ps.publishers {
				resolved = append(resolved, publisherPlatform)
			}
		} else {
			// Check if we have a publisher for this platform
			if _, exists := ps.publishers[platform]; exists {
				resolved = append(resolved, platform)
			} else {
				slog.Default().Warn("Publisher not available for platform", slog.String("platform", platform))
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, platform := range resolved {
		if !seen[platform] {
			unique = append(unique, platform)
			seen[platform] = true
		}
	}

	return unique, nil
}

// readPost reads the post's full article once for all platforms. A failure
// is not fatal: the prompt falls back to what the catalogue holds, or to the
// URL slug when the catalogue holds nothing either.
func (ps *PublishService) readPost(ctx context.Context, post *domain.Post) *domain.Post {
	if ps.reader == nil {
		return post
	}
	full, err := ps.reader.ReadPost(ctx, post)
	if err != nil || full == nil {
		attrs := []any{slog.String("uri", post.URI()), slog.Bool("catalogue_details", !post.Article().IsZero())}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}
		slog.Default().Warn("Could not read the post, generating from the catalogue entry only", attrs...)
		return post
	}
	article := full.Article()
	slog.Default().Info("Read post for content generation",
		slog.String("uri", full.URI()),
		slog.String("title", article.Title),
		slog.Int("body_chars", len([]rune(article.Body))),
		slog.Any("artists", article.Tags.Artists),
		slog.Any("places", article.Tags.Places))
	return full
}

// images returns the cover photographs of posts with their alt text, at most
// MaxPostImages, skipping posts without a cover. A describer that fails
// leaves the site's own alt text, or the title, in place: a photo with plain
// alt text beats no photo.
func (ps *PublishService) images(ctx context.Context, posts ...*domain.Post) []domain.Image {
	var images []domain.Image
	for _, post := range posts {
		if len(images) == domain.MaxPostImages {
			break
		}
		if _, ok := post.CoverImage(""); !ok {
			continue
		}
		description := ""
		if ps.describer != nil {
			cover := post.Article().Cover.URL
			var err error
			description, err = ps.describer.Describe(ctx, post, cover)
			if err != nil {
				slog.Default().Warn("Could not describe the cover photo, using the site's alt text",
					slog.String("image", cover), slog.String("error", err.Error()))
				description = ""
			}
		}
		image, _ := post.CoverImage(description)
		slog.Default().Info("Cover photo attached",
			slog.String("image", image.URL),
			slog.String("alt", image.Alt),
			slog.Bool("described", description != ""))
		images = append(images, image)
	}
	return images
}

// generateContentForPlatform generates content for a specific platform.
func (ps *PublishService) generateContentForPlatform(ctx context.Context, post *domain.Post, platform, dateKey string, dryRun bool, extra map[string]string) (*domain.GeneratedContent, error) {
	// Nil safety check
	if post == nil {
		return nil, errors.New("post cannot be nil")
	}

	req, err := ports.NewGenerationRequest([]*domain.Post{post}, platform, dateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create generation request: %w", err)
	}

	// Add anniversary context
	req.WithContext("anniversary_date", dateKey)
	req.WithContext("post_count", "1")
	for key, value := range extra {
		req.WithContext(key, value)
	}

	content, err := ps.generator.GenerateContent(ctx, req)
	if err != nil {
		if dryRun && strings.Contains(err.Error(), "not implemented") {
			// In dry-run mode, a placeholder generator returns "not implemented";
			// mock content stands in for the preview
			slog.Default().Info("DRY RUN - content generation simulated", slog.String("platform", platform))
			return ps.createMockContent(platform, post, dateKey), nil
		}
		return nil, err
	}

	return ps.reviewed(ctx, post, req, content)
}

// reviewed returns content once the critic approves it. A rejected draft is
// written again with the critic's findings, up to MaxRewrites times; when no
// draft passes, or the critic cannot give a verdict, the fallback template
// takes its place, because an unchecked claim about a real artist must not go
// out.
func (ps *PublishService) reviewed(ctx context.Context, post *domain.Post, req *ports.GenerationRequest, content *domain.GeneratedContent) (*domain.GeneratedContent, error) {
	if ps.critic == nil {
		return content, nil
	}
	logger := slog.Default().With(slog.String("platform", req.Platform), slog.String("uri", post.URI()))

	for rewrite := 0; ; rewrite++ {
		review, err := ps.critic.Review(ctx, post, content)
		if err != nil {
			logger.Warn("Critic gave no verdict, using the fallback template", slog.String("error", err.Error()))
			break
		}

		unsupported := make([]string, 0)
		for _, verdict := range review.Unsupported() {
			unsupported = append(unsupported, verdict.Claim)
		}
		logger.Info("Draft reviewed",
			slog.Int("rewrite", rewrite),
			slog.Bool("approved", review.Approved()),
			slog.Int("claims", len(review.Verdicts)),
			slog.Any("unsupported", unsupported),
			slog.Int("voice_score", review.VoiceScore),
			slog.String("voice_problems", review.VoiceProblems),
			slog.String("text", content.Text()))

		if review.Approved() {
			return content, nil
		}
		if rewrite == MaxRewrites {
			logger.Warn("No draft passed review, using the fallback template", slog.Int("rewrites", rewrite))
			break
		}

		req.WithContext(ports.ContextCriticFeedback, review.Feedback())
		content, err = ps.generator.GenerateContent(ctx, req)
		if err != nil {
			logger.Warn("Rewrite failed, using the fallback template", slog.String("error", err.Error()))
			break
		}
	}

	fallback, err := domain.FallbackContent(post, req.Platform)
	if err != nil {
		return nil, fmt.Errorf("fallback template failed: %w", err)
	}
	logger.Info("Fallback content", slog.String("text", fallback.Text()))
	return fallback, nil
}

// publishToPlatform publishes content to a specific platform.
func (ps *PublishService) publishToPlatform(ctx context.Context, platform string, content *domain.GeneratedContent, dryRun bool) (*domain.PlatformResult, error) {
	// Nil safety check
	if content == nil {
		return nil, errors.New("content cannot be nil")
	}

	publisher, exists := ps.publishers[platform]
	if !exists {
		return nil, fmt.Errorf("no publisher available for platform: %s", platform)
	}

	if dryRun {
		slog.Default().Info("DRY RUN - skipping actual publish", slog.String("platform", platform))

		// Validate content in dry-run mode, but expect "not implemented" error
		if err := publisher.ValidateContent(ctx, content); err != nil {
			if !strings.Contains(err.Error(), "not implemented") {
				return nil, fmt.Errorf("content validation failed: %w", err)
			}
		}

		// Create dry-run result with skipped status
		result, _ := domain.NewPlatformResult(platform, domain.StatusSkipped, content)
		result.SetSkipped("dry-run-"+platform, 0)
		return result, nil
	}

	// Validate content before publishing
	if err := publisher.ValidateContent(ctx, content); err != nil {
		result, _ := domain.NewPlatformResult(platform, domain.StatusFailed, content)
		result.SetFailure(fmt.Errorf("content validation failed: %w", err), 0)
		return result, err
	}

	// Perform actual publish
	startTime := time.Now()
	result, err := publisher.Publish(ctx, content)
	responseTime := time.Since(startTime)

	if err != nil {
		if result == nil {
			result, _ = domain.NewPlatformResult(platform, domain.StatusFailed, content)
		}
		result.SetFailure(err, responseTime)
		return result, err
	}

	return result, nil
}

// createMockContent creates mock content for dry-run mode.
func (ps *PublishService) createMockContent(platform string, post *domain.Post, dateKey string) *domain.GeneratedContent {
	// Nil safety check
	if post == nil {
		slog.Default().Warn("createMockContent called with nil post, using fallback")
		mockText := fmt.Sprintf("🎉 Anniversary post for %s! #UrbanArt #Anniversary", dateKey)
		content, _ := domain.NewGeneratedContent(mockText, platform, []string{"#UrbanArt", "#Anniversary"}, "mock-prompt-for-dry-run", "https://www.example.com/test")
		return content
	}

	mockText := fmt.Sprintf("🎉 Anniversary post for %s! Celebrating %d year(s) of Urban Art Core content. #UrbanArt #Anniversary",
		dateKey, time.Now().Year()-post.Date().Year())

	// Truncate if too long for platform
	limit := domain.BlueskyCharacterLimit
	if platform == domain.PlatformMastodon {
		limit = domain.MastodonCharacterLimit
	}

	if len(mockText) > limit {
		mockText = mockText[:limit-3] + "..."
	}

	hashtags := []string{"#UrbanArt", "#Anniversary", "#StreetArt"}
	content, _ := domain.NewGeneratedContent(mockText, platform, hashtags, "mock-prompt-for-dry-run", post.URI())

	return content
}
