package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/config"
	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
	refreshtokenrepo "github.com/johnquangdev/laverte-core/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-core/repository/user"
	"github.com/johnquangdev/laverte-core/util"
	"github.com/johnquangdev/laverte-core/util/oauth"
	"github.com/johnquangdev/laverte-core/util/tokenstore"
)

type UseCase struct {
	userRepo   userrepo.IRepository
	tokenRepo  refreshtokenrepo.IRepository
	oauth      oauth.IOAuthProvider
	tokenStore tokenstore.ITokenStore
	cfg        config.Config
	log        *zap.Logger
}

func New(
	userRepo userrepo.IRepository,
	tokenRepo refreshtokenrepo.IRepository,
	oauthSvc oauth.IOAuthProvider,
	tokenStore tokenstore.ITokenStore,
	cfg config.Config,
	log *zap.Logger,
) IUseCase {
	return &UseCase{userRepo: userRepo, tokenRepo: tokenRepo, oauth: oauthSvc, tokenStore: tokenStore, cfg: cfg, log: log}
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
		return nil, apperr.Unauthorized(errors.New("invalid or expired oauth state"))
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

func (uc *UseCase) PasswordLogin(ctx context.Context, req payload.PasswordLoginRequest) (*presenter.SessionResponse, error) {
	if uc.cfg.Environment != "development" {
		return nil, apperr.Unauthorized(errors.New("password login disabled"))
	}
	if strings.TrimSpace(uc.cfg.LocalAdminPassword) == "" {
		return nil, apperr.Unauthorized(errors.New("password login disabled"))
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" ||
		subtle.ConstantTimeCompare([]byte(username), []byte(uc.cfg.LocalAdminUsername)) != 1 ||
		subtle.ConstantTimeCompare([]byte(password), []byte(uc.cfg.LocalAdminPassword)) != 1 {
		return nil, apperr.Unauthorized(errors.New("invalid credentials"))
	}

	user, err := uc.userRepo.GetByOAuth(ctx, "local", username)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		displayName := username
		user = &model.User{
			Email:         fmt.Sprintf("%s@laverte-home.local", username),
			Name:          &displayName,
			OAuthProvider: "local",
			OAuthID:       username,
			Role:          model.RoleAdmin,
		}
		if err := uc.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
	} else if user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		now := time.Now()
		if err := uc.userRepo.SetRole(ctx, user.ID, model.RoleAdmin, nil, &now); err != nil {
			return nil, err
		}
		user.Role = model.RoleAdmin
	}

	return uc.issueTokenPair(ctx, user, uuid.New().String())
}

func (uc *UseCase) RefreshToken(ctx context.Context, refreshTokenStr string) (*presenter.SessionResponse, error) {
	claims, err := util.ParseToken(uc.cfg.JWTRefreshSecret, refreshTokenStr)
	if err != nil {
		return nil, apperr.Unauthorized(errors.New("invalid refresh token"))
	}

	stored, err := uc.tokenRepo.GetByTokenID(ctx, claims.TokenID)
	if err != nil || stored.RevokedAt != nil {
		if err == nil {
			// Replaying a revoked token means it leaked, so the whole family dies
			// with it. A failure here leaves the leaked family live, which is the
			// exact scenario this branch exists for — it must not pass silently.
			if rerr := uc.tokenRepo.RevokeFamily(ctx, stored.FamilyID); rerr != nil {
				uc.log.Error("refresh-token reuse detected but family revoke failed",
					zap.String("family_id", stored.FamilyID),
					zap.Uint("user_id", claims.UserID),
					zap.Error(rerr))
			}
		}
		return nil, apperr.Unauthorized(errors.New("refresh token revoked or not found"))
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
