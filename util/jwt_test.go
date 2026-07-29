package util

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndParseToken(t *testing.T) {
	claims := Claims{UserID: 42, TokenID: "tok-1", FamilyID: "fam-1"}
	tokenStr, err := GenerateToken("secret", claims, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	got, err := ParseToken("secret", tokenStr)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if got.UserID != 42 || got.TokenID != "tok-1" || got.FamilyID != "fam-1" {
		t.Errorf("ParseToken() = %+v", got)
	}
}

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	tokenStr, _ := GenerateToken("secret-a", Claims{UserID: 1}, time.Hour)
	if _, err := ParseToken("secret-b", tokenStr); err == nil {
		t.Error("ParseToken() with wrong secret = nil error, want error")
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	tokenStr, _ := GenerateToken("secret", Claims{UserID: 1}, -time.Minute)
	if _, err := ParseToken("secret", tokenStr); err == nil {
		t.Error("ParseToken() with expired token = nil error, want error")
	}
}

// ParseToken must reject a token whose header claims a non-HMAC algorithm.
// Without the signing-method check, an attacker could present an unsigned
// (alg=none) token and have its claims trusted.
func TestParseTokenRejectsNonHMACAlgorithm(t *testing.T) {
	claims := Claims{UserID: 99, TokenID: "forged"}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building alg=none token: %v", err)
	}

	if _, err := ParseToken("secret", unsigned); err == nil {
		t.Error("ParseToken() accepted an alg=none token, want rejection")
	}
}
