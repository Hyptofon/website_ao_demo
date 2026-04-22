package auth

import (
	"testing"
	"time"
)

func TestGenerateAndValidateToken(t *testing.T) {
	svc := NewJWTService("test-secret-32-chars-long-enough")

	user := &GoogleUserInfo{
		Email:   "admin@university.edu.ua",
		Name:    "Test Admin",
		Picture: "https://example.com/pic.jpg",
	}

	token, err := svc.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.Email != user.Email {
		t.Errorf("Expected email %q, got %q", user.Email, claims.Email)
	}
	if claims.TokenType != "access" {
		t.Errorf("Expected token type 'access', got %q", claims.TokenType)
	}
}

func TestGenerateAndValidateRefreshToken(t *testing.T) {
	svc := NewJWTService("test-secret-32-chars-long-enough")

	user := &GoogleUserInfo{
		Email: "admin@university.edu.ua",
		Name:  "Test Admin",
	}

	refreshToken, err := svc.GenerateRefreshToken(user)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}

	// Should validate as refresh token
	claims, err := svc.ValidateRefreshToken(refreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}
	if claims.Email != user.Email {
		t.Errorf("Expected email %q, got %q", user.Email, claims.Email)
	}
	if claims.TokenType != "refresh" {
		t.Errorf("Expected token type 'refresh', got %q", claims.TokenType)
	}

	// Should NOT validate as access token
	_, err = svc.ValidateToken(refreshToken)
	if err == nil {
		t.Error("Expected error when validating refresh token as access token")
	}
}

func TestAccessTokenCannotBeUsedAsRefresh(t *testing.T) {
	svc := NewJWTService("test-secret-32-chars-long-enough")

	user := &GoogleUserInfo{Email: "admin@university.edu.ua"}
	accessToken, _ := svc.GenerateToken(user)

	_, err := svc.ValidateRefreshToken(accessToken)
	if err == nil {
		t.Error("Expected error when using access token as refresh token")
	}
}

func TestExpiredToken(t *testing.T) {
	svc := NewJWTService("test-secret-32-chars-long-enough")

	claims := Claims{
		Email:     "admin@test.com",
		TokenType: "access",
		IssuedAt:  time.Now().Add(-25 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	}

	token, err := svc.signClaims(claims)
	if err != nil {
		t.Fatalf("signClaims failed: %v", err)
	}

	_, err = svc.ValidateToken(token)
	if err == nil {
		t.Error("Expected error for expired token")
	}
}

func TestRefreshTokenExpiry(t *testing.T) {
	svc := NewJWTService("test-secret")

	user := &GoogleUserInfo{Email: "admin@test.com"}
	token, _ := svc.GenerateRefreshToken(user)

	claims, _ := svc.ValidateRefreshToken(token)
	expectedExpiry := time.Now().Add(RefreshTokenExpiry).Unix()

	// Allow 5 second tolerance
	diff := claims.ExpiresAt - expectedExpiry
	if diff < -5 || diff > 5 {
		t.Errorf("Refresh token expiry should be ~30 days, got diff %d seconds", diff)
	}
}
