package util

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"user_id"`
	TokenID  string `json:"token_id"`
	FamilyID string `json:"family_id"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, claims Claims, ttl time.Duration) (string, error) {
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(ttl))
	claims.IssuedAt = jwt.NewNumericDate(time.Now())
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
