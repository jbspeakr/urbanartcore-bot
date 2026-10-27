package ports

import "context"

// Article holds what the bot can learn about a blog post from its web page.
// Every field may be empty: an old post can lack a description or body text.
type Article struct {
	Title       string // Headline of the post
	Description string // Teaser from the page's meta tags
	Excerpt     string // Opening paragraphs of the post body, trimmed
}

// ArticleReader reads a blog post's web page, so generated posts can talk
// about what the post shows instead of guessing from its URL.
type ArticleReader interface {
	// ReadArticle fetches the page at uri and extracts its title,
	// description and opening paragraphs.
	ReadArticle(ctx context.Context, uri string) (*Article, error)
}
