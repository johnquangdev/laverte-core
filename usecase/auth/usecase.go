package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	refreshtokenrepo "github.com/johnquangdev/laverte-home/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
	"github.com/johnquangdev/laverte-home/util"
	"github.com/johnquangdev/laverte-home/util/oauth"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type UseCase struct {
	userRepo   userrepo.IRepository
	tokenRepo  refreshtokenrepo.IRepository
	oauth      oauth.IOAuthProvider
	tokenStore tokenstore.ITokenStore
	cfg        config.Config
}

func New(
	userRepo userrepo.IRepository,
	tokenRepo refreshtokenrepo.IRepository,
	oauthSvc oauth.IOAuthProvider,
	tokenStore tokenstore.ITokenStore,
	cfg config.Config,
) IUseCase {
	return &UseCase{userRepo: userRepo, tokenRepo: tokenRepo, oauth: oauthSvc, tokenStore: tokenStore, cfg: cfg}
}

func (uc *UseCase) LoginURL(ctx context.Context) (*presenter.GoogleLoginURLResponse, error) {
	result, err := uc.oauth.AuthorizeURL(ctx)
	if err != nil {
		return nil, err
	}
	if err := uc.tokenStore.SaveState(ctx, result.State); err != nil {
		return nil, err
	}
	return &presenter.GoogleLoginURLResponse{URL: result.URL}, nil
}

func (uc *UseCase) Callback(ctx context.Context, req payload.GoogleCallbackRequest) (*presenter.SessionResponse, error) {
	ok, err := uc.tokenStore.ValidateState(ctx, req.State)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("invalid or expired oauth state")
	}

	info, err := uc.oauth.Exchange(ctx, req.Code, req.State)
	if err != nil {
		return nil, err
	}

	user, err := uc.userRepo.GetByOAuth(ctx, info.Provider, info.OAuthID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		user = &model.User{Email: info.Email, OAuthProvider: info.Provider, OAuthID: info.OAuthID}
		if err := uc.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
	}

	return uc.issueTokenPair(ctx, user, uuid.New().String())
}

func (uc *UseCase) RefreshToken(ctx context.Context, refreshTokenStr string) (*presenter.SessionResponse, error) {
	claims, err := util.ParseToken(uc.cfg.JWTRefreshSecret, refreshTokenStr)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	stored, err := uc.tokenRepo.GetByTokenID(ctx, claims.TokenID)
	if err != nil || stored.RevokedAt != nil {
		if err == nil {
			_ = uc.tokenRepo.RevokeFamily(ctx, stored.FamilyID)
		}
		return nil, errors.New("refresh token revoked or not found")
	}

	if err = uc.tokenRepo.Revoke(ctx, claims.TokenID); err != nil {
		return nil, err
	}

	user, err := uc.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	return uc.issueTokenPair(ctx, user, stored.FamilyID)
}

func (uc *UseCase) Logout(ctx context.Context, authHeader string) error {
	// An unparsable or already-expired access token has nothing to blacklist,
	// so logout with a bad token still reports success rather than an error.
	tokenID, ok := parseAccessTokenID(uc.cfg.JWTAccessSecret, authHeader)
	if !ok {
		return nil
	}
	return uc.tokenStore.BlacklistToken(ctx, tokenID)
}

func parseAccessTokenID(secret, authHeader string) (string, bool) {
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := util.ParseToken(secret, tokenStr)
	if err != nil {
		return "", false
	}
	return claims.TokenID, true
}

func (uc *UseCase) issueTokenPair(ctx context.Context, user *model.User, familyID string) (*presenter.SessionResponse, error) {
	accessTokenID := uuid.New().String()
	refreshTokenID := uuid.New().String()

	accessTTL := time.Duration(uc.cfg.JWTAccessTTLMinutes) * time.Minute
	refreshTTL := time.Duration(uc.cfg.JWTRefreshTTLDays) * 24 * time.Hour

	accessToken, err := util.GenerateToken(uc.cfg.JWTAccessSecret, util.Claims{UserID: user.ID, TokenID: accessTokenID, FamilyID: familyID}, accessTTL)
	if err != nil {
		return nil, err
	}
	refreshToken, err := util.GenerateToken(uc.cfg.JWTRefreshSecret, util.Claims{UserID: user.ID, TokenID: refreshTokenID, FamilyID: familyID}, refreshTTL)
	if err != nil {
		return nil, err
	}
	if err := uc.tokenRepo.Create(ctx, &model.RefreshToken{TokenID: refreshTokenID, UserID: user.ID, FamilyID: familyID}); err != nil {
		return nil, err
	}

	return &presenter.SessionResponse{
		AccessToken: accessToken, RefreshToken: refreshToken,
		ExpiresIn: uc.cfg.JWTAccessTTLMinutes * 60, TokenType: "Bearer",
		User: presenter.ToUserResponse(user),
	}, nil
}
