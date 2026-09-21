package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/johnquangdev/laverte-core/config"
)

type googleOAuth struct {
	oc *oauth2.Config
}

func NewGoogle(cfg *config.Config) IOAuthProvider {
	return &googleOAuth{
		oc: &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURI,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (g *googleOAuth) AuthorizeURL(_ context.Context) (AuthorizeURLResult, error) {
	state := uuid.New().String()
	url := g.oc.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return AuthorizeURLResult{URL: url, State: state}, nil
}

type googleUserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
}

func (g *googleOAuth) Exchange(ctx context.Context, code, _ string) (ExchangeResult, error) {
	token, err := g.oc.Exchange(ctx, code)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: exchange failed: %w", err)
	}

	client := g.oc.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: userinfo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: read userinfo body: %w", err)
	}

	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: parse userinfo: %w", err)
	}

	return ExchangeResult{Email: info.Email, OAuthID: info.Sub, Provider: "google", Avatar: info.Picture}, nil
}
