// internal/access/scoped_token.go
package access

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

var (
	ErrTokenExpired  = errors.New("access: token expired")
	ErrTokenInvalid  = errors.New("access: token invalid")
	ErrTokenMismatch = errors.New("access: HMAC mismatch")
)

// TokenSigner generates and verifies HMAC-SHA256 scoped media tokens.
type TokenSigner struct {
	currentKey []byte
	prevKey    []byte
	ttl        time.Duration
}

const (
	tokenVersion = "v1"
	hkdfSalt     = "goclaw-media-token-v1"
	hkdfInfo     = "hmac-sha256"
	minKeyLen    = 32
)

// NewTokenSigner creates a signer. Panics if primaryKey < 32 bytes.
func NewTokenSigner(primaryKey, prevKey string, ttl time.Duration) *TokenSigner {
	if len(primaryKey) < minKeyLen {
		panic(fmt.Sprintf("access: signing key must be >= %d bytes, got %d", minKeyLen, len(primaryKey)))
	}
	s := &TokenSigner{
		currentKey: deriveKey([]byte(primaryKey)),
		ttl:        ttl,
	}
	if len(prevKey) >= minKeyLen {
		s.prevKey = deriveKey([]byte(prevKey))
	}
	return s
}

func deriveKey(master []byte) []byte {
	r := hkdf.New(sha256.New, master, []byte(hkdfSalt), []byte(hkdfInfo))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		panic("access: HKDF derive failed: " + err.Error())
	}
	return key
}

// Generate creates a scoped token for a media ID and session hash.
// Format: base64url(v1:mediaID:sessionHash:expiryUnixMilli).base64url(hmac)
func (s *TokenSigner) Generate(mediaID, sessionHash string) (string, error) {
	expiry := time.Now().Add(s.ttl).UnixMilli()
	payload := fmt.Sprintf("%s:%s:%s:%d", tokenVersion, mediaID, sessionHash, expiry)
	sig := s.sign(s.currentKey, payload)

	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sigEncoded := base64.RawURLEncoding.EncodeToString(sig)

	return encoded + "." + sigEncoded, nil
}

// Verify validates a scoped token and returns the session hash.
// Tries current key first, then previous key for graceful rotation.
func (s *TokenSigner) Verify(token, expectedMediaID string) (sessionHash string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", ErrTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrTokenInvalid
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrTokenInvalid
	}

	payload := string(payloadBytes)

	if !s.verifyHMAC(s.currentKey, payload, sigBytes) {
		if s.prevKey == nil || !s.verifyHMAC(s.prevKey, payload, sigBytes) {
			return "", ErrTokenMismatch
		}
	}

	fields := strings.SplitN(payload, ":", 4)
	if len(fields) != 4 || fields[0] != tokenVersion {
		return "", ErrTokenInvalid
	}
	if fields[1] != expectedMediaID {
		return "", ErrTokenMismatch
	}

	expiry, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return "", ErrTokenInvalid
	}
	if time.Now().UnixMilli() > expiry {
		return "", ErrTokenExpired
	}

	return fields[2], nil
}

func (s *TokenSigner) sign(key []byte, payload string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func (s *TokenSigner) verifyHMAC(key []byte, payload string, sig []byte) bool {
	expected := s.sign(key, payload)
	return hmac.Equal(expected, sig)
}

// IsScopedToken returns true if the token contains a dot separator
// (gateway tokens never contain dots).
func IsScopedToken(token string) bool {
	return strings.Contains(token, ".")
}
