// internal/auth/jwt.go
package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidatorConfig configures the JWT validator.
type JWTValidatorConfig struct {
	JWKSURL  string // Keycloak JWKS endpoint (e.g. https://auth.example.com/realms/vnpay/protocol/openid-connect/certs)
	Issuer   string // Expected issuer (e.g. https://auth.example.com/realms/vnpay)
	Audience string // Expected audience (e.g. goclaw-gateway)
}

// JWTValidator validates Keycloak-issued JWT tokens using RS256 + JWKS.
type JWTValidator struct {
	cfg     JWTValidatorConfig
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey // kid → public key
	fetched time.Time                // last JWKS fetch time
}

// NewJWTValidator creates a new JWT validator with the given config.
func NewJWTValidator(cfg JWTValidatorConfig) *JWTValidator {
	return &JWTValidator{
		cfg:  cfg,
		keys: make(map[string]*rsa.PublicKey),
	}
}

// jwksCacheTTL is how long JWKS keys are cached before re-fetching.
const jwksCacheTTL = 10 * time.Minute

// Validate parses and validates a JWT token string, returning extracted claims.
func (v *JWTValidator) Validate(tokenStr string) (*KeycloakClaims, error) {
	token, err := jwt.Parse(tokenStr, v.keyFunc,
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt validation failed: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("jwt: token is not valid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("jwt: unexpected claims type")
	}

	return extractKeycloakClaims(claims), nil
}

// keyFunc is the jwt.Keyfunc that resolves the signing key by kid.
func (v *JWTValidator) keyFunc(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errors.New("jwt: missing kid header")
	}

	// Try cached key first
	v.mu.RLock()
	key, found := v.keys[kid]
	fresh := time.Since(v.fetched) < jwksCacheTTL
	v.mu.RUnlock()

	if found && fresh {
		return key, nil
	}

	// Fetch JWKS (or re-fetch if stale / kid not found)
	if err := v.fetchJWKS(); err != nil {
		// If we have a cached key and fetch failed, use cached (graceful degradation)
		if found {
			slog.Warn("jwt: JWKS fetch failed, using cached key", "kid", kid, "error", err)
			return key, nil
		}
		return nil, fmt.Errorf("jwt: JWKS fetch failed and no cached key for kid %s: %w", kid, err)
	}

	v.mu.RLock()
	key, found = v.keys[kid]
	v.mu.RUnlock()
	if !found {
		return nil, fmt.Errorf("jwt: no key found for kid %s", kid)
	}
	return key, nil
}

// jwksResponse represents the JWKS endpoint response.
type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// fetchJWKS fetches JWKS from the configured endpoint and updates the key cache.
func (v *JWTValidator) fetchJWKS() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(v.cfg.JWKSURL)
	if err != nil {
		return fmt.Errorf("JWKS HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS returned status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("JWKS decode failed: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pubKey, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			slog.Warn("jwt: failed to parse JWKS key", "kid", k.Kid, "error", err)
			continue
		}
		newKeys[k.Kid] = pubKey
	}

	if len(newKeys) == 0 {
		return errors.New("JWKS: no valid RSA keys found")
	}

	v.mu.Lock()
	v.keys = newKeys
	v.fetched = time.Now()
	v.mu.Unlock()

	slog.Debug("jwt: JWKS keys refreshed", "count", len(newKeys))
	return nil
}

// parseRSAPublicKey parses base64url-encoded N and E into an rsa.PublicKey.
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode E: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// extractKeycloakClaims extracts typed claims from a validated JWT MapClaims.
func extractKeycloakClaims(claims jwt.MapClaims) *KeycloakClaims {
	kc := &KeycloakClaims{}

	if sub, ok := claims["sub"].(string); ok {
		kc.KeycloakID = sub
	}
	if email, ok := claims["email"].(string); ok {
		kc.Email = email
	}
	if name, ok := claims["name"].(string); ok {
		kc.Name = name
	}

	// realm_roles: custom claim mapped in Keycloak client (explicit mapper)
	if roles, ok := claims["realm_roles"].([]any); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok {
				kc.RealmRoles = append(kc.RealmRoles, s)
			}
		}
	}
	// realm_access.roles: standard Keycloak JWT structure (fallback when no custom mapper)
	if len(kc.RealmRoles) == 0 {
		if realmAccess, ok := claims["realm_access"].(map[string]any); ok {
			if roles, ok := realmAccess["roles"].([]any); ok {
				for _, r := range roles {
					if s, ok := r.(string); ok {
						kc.RealmRoles = append(kc.RealmRoles, s)
					}
				}
			}
		}
	}

	// groups: from Keycloak group membership mapper
	if groups, ok := claims["groups"].([]any); ok {
		for _, g := range groups {
			if s, ok := g.(string); ok {
				kc.Groups = append(kc.Groups, s)
			}
		}
	}

	// identity_provider: which IdP was used (google, microsoft, etc.)
	if idp, ok := claims["identity_provider"].(string); ok {
		kc.AuthProvider = idp
	}

	return kc
}
