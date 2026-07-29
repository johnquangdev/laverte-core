package presenter

import "github.com/johnquangdev/laverte-home/model"

type GoogleLoginURLResponse struct {
	URL string `json:"url"`
}

type UserResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type SessionResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         UserResponse `json:"user"`
}

func ToUserResponse(u *model.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Role: u.Role}
}
