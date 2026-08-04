package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OIDCValidator validates RS256 JWTs from a configured OIDC provider by fetching
// and caching JWKS keys. It returns the best-effort user identifier to inject
// into Session Manager request headers.
type OIDCValidator struct {
	issuer   string
	jwksURL  string
	clientID string

	httpClient *http.Client

	mu       sync.RWMutex
	keys     map[string]*rsa.PublicKey
	loadedAt time.Time
}

type oidcClaims struct {
	jwt.RegisteredClaims
	PreferredUsername string `json:"preferred_username,omitempty"`
	Email             string `json:"email,omitempty"`
}

func NewOIDCValidator(issuer, jwksURL, clientID string) *OIDCValidator {
	issuer = strings.TrimSpace(issuer)
	issuer = strings.TrimRight(issuer, "/")

	return &OIDCValidator{
		issuer:      issuer,
		jwksURL:     strings.TrimSpace(jwksURL),
		clientID:    strings.TrimSpace(clientID),
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		keys:        map[string]*rsa.PublicKey{},
		loadedAt:    time.Time{},
	}
}

func (v *OIDCValidator) Verify(ctx context.Context, rawToken string) (string, error) {
	if v.issuer == "" {
		return "", fmt.Errorf("oidc issuer not configured")
	}

	claims := &oidcClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		kid, _ := token.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	if err != nil || !parsed.Valid {
		return "", fmt.Errorf("verify JWT: %w", err)
	}

	if v.clientID != "" && len(claims.Audience) > 0 {
		matched := false
		for _, aud := range claims.Audience {
			if aud == v.clientID {
				matched = true
				break
			}
		}
		if !matched {
			return "", fmt.Errorf("token audience does not match expected client")
		}
	}

	if claims.Subject == "" {
		return "", fmt.Errorf("token subject missing")
	}
	if claims.PreferredUsername != "" {
		return claims.PreferredUsername, nil
	}
	return claims.Subject, nil
}

func (v *OIDCValidator) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key := v.keys[kid]
	fresh := time.Since(v.loadedAt) < time.Hour
	v.mu.RUnlock()

	if key != nil && fresh {
		return key, nil
	}

	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key = v.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("JWKS key %q not found", kid)
	}
	return key, nil
}

func (v *OIDCValidator) refresh(ctx context.Context) error {
	jwksURL := v.jwksURL
	if jwksURL == "" {
		// OpenID discovery is simple and avoids needing provider-specific JWKS paths.
		discoveryURL := v.issuer + "/.well-known/openid-configuration"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
		if err != nil {
			return fmt.Errorf("build discovery request: %w", err)
		}
		resp, err := v.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch OIDC discovery: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch OIDC discovery: %s", resp.Status)
		}

		var doc struct {
			JwksURI string `json:"jwks_uri"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			return fmt.Errorf("decode discovery JSON: %w", err)
		}
		if doc.JwksURI == "" {
			return fmt.Errorf("discovery returned empty jwks_uri")
		}
		jwksURL = doc.JwksURI
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build JWKS request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: %s", resp.Status)
	}

	// Key format matches standard JWKS.
	var document struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return fmt.Errorf("decode JWKS JSON: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.Kty != "RSA" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil {
			continue
		}

		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 + int(value)
		}

		keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return fmt.Errorf("JWKS contained no usable RSA keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.loadedAt = time.Now()
	v.mu.Unlock()
	return nil
}

