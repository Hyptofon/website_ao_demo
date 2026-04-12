package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ─── Minimal JWT (HS256) ────────────────────────────────────────────────────
// Self-contained JWT implementation — no external dependencies.
// Chosen over a library to minimize attack surface and keep the binary lean.
// Production-grade: constant-time signature comparison, proper claims validation.

const (
	// TokenExpiry is the access token lifetime.
	TokenExpiry = 24 * time.Hour
	jwtAlg      = "HS256"
)

// Claims represents the JWT payload.
type Claims struct {
	Email   string `json:"email"`
	Name    string `json:"name,omitempty"`
	Picture string `json:"picture,omitempty"`
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
}

// IsExpired returns true if the token has expired.
func (c *Claims) IsExpired() bool {
	return time.Now().Unix() > c.ExpiresAt
}

// JWTService handles JWT creation and validation.
type JWTService struct {
	secret []byte
}

// NewJWTService creates a new JWT service with the given secret.
func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// GenerateToken creates a signed JWT for the given user info.
func (s *JWTService) GenerateToken(user *GoogleUserInfo) (string, error) {
	now := time.Now()
	claims := Claims{
		Email:     user.Email,
		Name:      user.Name,
		Picture:   user.Picture,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(TokenExpiry).Unix(),
	}

	// Header
	header := map[string]string{"alg": jwtAlg, "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64URLEncode(headerJSON)

	// Payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: marshal claims: %w", err)
	}
	payloadB64 := base64URLEncode(payloadJSON)

	// Signature
	signingInput := headerB64 + "." + payloadB64
	signature := s.sign([]byte(signingInput))
	signatureB64 := base64URLEncode(signature)

	return signingInput + "." + signatureB64, nil
}

// ValidateToken parses and validates a JWT string. Returns claims on success.
func (s *JWTService) ValidateToken(tokenStr string) (*Claims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt: invalid token format")
	}

	// Verify signature (constant-time comparison)
	signingInput := parts[0] + "." + parts[1]
	expectedSig := s.sign([]byte(signingInput))
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid signature encoding: %w", err)
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("jwt: invalid signature")
	}

	// Decode payload
	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid payload encoding: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("jwt: invalid payload: %w", err)
	}

	// Check expiry
	if claims.IsExpired() {
		return nil, fmt.Errorf("jwt: token expired")
	}

	if claims.Email == "" {
		return nil, fmt.Errorf("jwt: missing email in claims")
	}

	return &claims, nil
}

// sign creates an HMAC-SHA256 signature.
func (s *JWTService) sign(data []byte) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(data)
	return mac.Sum(nil)
}

// base64URLEncode encodes data using base64url (no padding).
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64URLDecode decodes base64url-encoded data.
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
