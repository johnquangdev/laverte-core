package auth

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	LoginURL(ctx context.Context) (*presenter.GoogleLoginURLResponse, error)
	Callback(ctx context.Context, req payload.GoogleCallbackRequest) (*presenter.SessionResponse, error)
	PasswordLogin(ctx context.Context, req payload.PasswordLoginRequest) (*presenter.SessionResponse, error)
	RefreshToken(ctx context.Context, refreshTokenStr string) (*presenter.SessionResponse, error)
	Logout(ctx context.Context, authHeader string) error
}
