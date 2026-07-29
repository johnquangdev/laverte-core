package util

import (
	"strings"
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
//
// Asserting only "some error came back" would not test our own guard at all:
// jwt/v5 independently refuses alg=none, so such a test passes with the
// signing-method check deleted. Assert on the guard's own message instead —
// with the check present the error reads "unexpected signing method"; without
// it, the library's own "'none' signature type is not allowed" surfaces and
// this test fails, which is the point.
func TestParseTokenRejectsNonHMACAlgorithm(t *testing.T) {
	claims := Claims{UserID: 99, TokenID: "forged"}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building alg=none token: %v", err)
	}

	_, err = ParseToken("secret", unsigned)
	if err == nil {
		t.Fatal("ParseToken() accepted an alg=none token, want rejection")
	}
	if !strings.Contains(err.Error(), "unexpected signing method") {
		t.Errorf("rejection came from somewhere other than our signing-method guard: %v", err)
	}
}
