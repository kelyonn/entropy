package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgAuth "github.com/entropy/pkg/auth"
	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCValidatorVerifiesSignatureIssuerAndClientAudience(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kid": kid,
				"kty": "RSA",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes()),
			}},
		})
	}))
	defer server.Close()

	validator := pkgAuth.NewOIDCValidator(server.URL, server.URL, "web-client")

	claims := struct {
		jwt.RegisteredClaims
		PreferredUsername string `json:"preferred_username,omitempty"`
	}{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    server.URL,
			Subject:   "user-123",
			Audience:  jwt.ClaimStrings{"web-client"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	subject, err := validator.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if subject != "user-123" {
		t.Fatalf("unexpected subject %q", subject)
	}
}
