// internal/auth/jwt_test.go
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTValidator_ValidToken(t *testing.T) {
	// Generate RSA key pair for testing
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	// Create JWKS server
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": "test-key-1",
					"n":   base64.RawURLEncoding.EncodeToString(privKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privKey.PublicKey.E)).Bytes()),
				},
			},
		}
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	validator := NewJWTValidator(JWTValidatorConfig{
		JWKSURL:  jwksServer.URL,
		Issuer:   "https://auth.example.com/realms/vnpay",
		Audience: "goclaw-gateway",
	})

	// Create valid token
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":         "550e8400-e29b-41d4-a716-446655440000",
		"email":       "test@company.com",
		"name":        "Test User",
		"realm_roles": []string{"member"},
		"groups":      []string{"Engineering"},
		"iss":         "https://auth.example.com/realms/vnpay",
		"aud":         "goclaw-gateway",
		"exp":         now.Add(1 * time.Hour).Unix(),
		"iat":         now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"
	tokenStr, err := token.SignedString(privKey)
	if err != nil {
		t.Fatal(err)
	}

	// Validate
	result, err := validator.Validate(tokenStr)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if result.KeycloakID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected keycloak ID 550e..., got %s", result.KeycloakID)
	}
	if result.Email != "test@company.com" {
		t.Errorf("expected email test@company.com, got %s", result.Email)
	}
	if result.Name != "Test User" {
		t.Errorf("expected name Test User, got %s", result.Name)
	}
	if len(result.RealmRoles) != 1 || result.RealmRoles[0] != "member" {
		t.Errorf("expected realm_roles [member], got %v", result.RealmRoles)
	}
}

func TestJWTValidator_ExpiredToken(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": "test-key-1",
					"n":   base64.RawURLEncoding.EncodeToString(privKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privKey.PublicKey.E)).Bytes()),
				},
			},
		}
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	validator := NewJWTValidator(JWTValidatorConfig{
		JWKSURL:  jwksServer.URL,
		Issuer:   "https://auth.example.com/realms/vnpay",
		Audience: "goclaw-gateway",
	})

	// Create expired token
	claims := jwt.MapClaims{
		"sub":   "550e8400-e29b-41d4-a716-446655440000",
		"email": "test@company.com",
		"iss":   "https://auth.example.com/realms/vnpay",
		"aud":   "goclaw-gateway",
		"exp":   time.Now().Add(-1 * time.Hour).Unix(),
		"iat":   time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"
	tokenStr, _ := token.SignedString(privKey)

	_, err = validator.Validate(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestJWTValidator_WrongIssuer(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": "test-key-1",
					"n":   base64.RawURLEncoding.EncodeToString(privKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privKey.PublicKey.E)).Bytes()),
				},
			},
		}
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	validator := NewJWTValidator(JWTValidatorConfig{
		JWKSURL:  jwksServer.URL,
		Issuer:   "https://auth.example.com/realms/vnpay",
		Audience: "goclaw-gateway",
	})

	claims := jwt.MapClaims{
		"sub":   "550e8400-e29b-41d4-a716-446655440000",
		"email": "test@company.com",
		"iss":   "https://evil.example.com/realms/wrong",
		"aud":   "goclaw-gateway",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"
	tokenStr, _ := token.SignedString(privKey)

	_, err = validator.Validate(tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
}
