package oauth

import "context"

type AuthorizeURLResult struct {
	URL   string
	State string
}

type ExchangeResult struct {
	Email    string
	OAuthID  string
	Provider string
	Avatar   string
}

// IOAuthProvider abstracts OAuth2 provider (Google).
type IOAuthProvider interface {
	AuthorizeURL(ctx context.Context) (AuthorizeURLResult, error)
	Exchange(ctx context.Context, code, state string) (ExchangeResult, error)
}
