package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	pkgAuth "github.com/entropy/pkg/auth"
)

type contextKey string

const UserIDKey contextKey = "userId"

// AuthMiddleware validates OIDC JWT when configured; guest path uses X-Guest header.
func AuthMiddleware(next http.Handler) http.Handler {
	issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL"))
	jwksURL := strings.TrimSpace(os.Getenv("OIDC_JWKS_URL"))
	clientID := strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))

	// No configured OIDC provider => keep guest/local behavior.
	if issuer == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	validator := pkgAuth.NewOIDCValidator(issuer, jwksURL, clientID)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" || strings.HasPrefix(r.URL.Path, "/internal/") {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		} else if r.URL.Path == "/ws" || r.URL.Path == "/ws/pty" {
			token = r.URL.Query().Get("access_token")
		}

		if token != "" {
			userID, err := validator.Verify(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			r.Header.Set("X-User-Id", userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		guest := r.Header.Get("X-Guest") == "1" || r.URL.Query().Get("guest") == "1"
		if guest && r.URL.Path != "/api/sessions" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/dashboard" {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "authentication required", http.StatusUnauthorized)
	})
}
