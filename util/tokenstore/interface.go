package tokenstore

import "context"

// ITokenStore backs OAuth CSRF-state single-use checks and JWT-blacklist-on-logout.
type ITokenStore interface {
	SaveState(ctx context.Context, state string) error
	ValidateState(ctx context.Context, state string) (bool, error)
	BlacklistToken(ctx context.Context, tokenID string) error
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}
