package auth

import (
	"testing"
	"time"
)

func TestJWT_GenerateAndValidate(t *testing.T) {
	svc := NewJWTService("test-secret-key-for-jwt-32bytes!")
	user := &GoogleUserInfo{
		Email:   "admin@oa.edu.ua",
		Name:    "Test Admin",
		Picture: "https://example.com/photo.jpg",
	}

	token, err := svc.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Token should have 3 parts
	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.Email != "admin@oa.edu.ua" {
		t.Errorf("expected email=admin@oa.edu.ua, got %s", claims.Email)
	}
	if claims.Name != "Test Admin" {
		t.Errorf("expected name=Test Admin, got %s", claims.Name)
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-for-jwt-32bytes!")

	// Manually create an expired token by setting exp to the past
	user := &GoogleUserInfo{Email: "test@test.com"}
	token, _ := svc.GenerateToken(user)

	// We can't easily make it expired without modifying the claim,
	// so let's test the IsExpired logic directly
	claims := &Claims{
		Email:     "test@test.com",
		IssuedAt:  time.Now().Add(-48 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-24 * time.Hour).Unix(),
	}

	if !claims.IsExpired() {
		t.Error("claim should be expired")
	}

	// Valid token should NOT be expired
	validClaims, _ := svc.ValidateToken(token)
	if validClaims.IsExpired() {
		t.Error("freshly generated token should not be expired")
	}
}

func TestJWT_WrongSecret(t *testing.T) {
	svc1 := NewJWTService("secret-one-aaaaaaaaaaaaaaaa")
	svc2 := NewJWTService("secret-two-bbbbbbbbbbbbbbbb")

	user := &GoogleUserInfo{Email: "test@test.com"}
	token, _ := svc1.GenerateToken(user)

	_, err := svc2.ValidateToken(token)
	if err == nil {
		t.Error("expected error when validating with wrong secret")
	}
}

func TestJWT_MalformedToken(t *testing.T) {
	svc := NewJWTService("test-secret")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one_part", "abc"},
		{"two_parts", "abc.def"},
		{"invalid_base64", "abc.def.!!!"},
		{"tampered_payload", "eyJhbGciOiJIUzI1NiJ9.dGVzdA.abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tt.token)
			if err == nil {
				t.Errorf("expected error for malformed token %q", tt.token)
			}
		})
	}
}

func TestJWT_MissingEmail(t *testing.T) {
	svc := NewJWTService("test-secret")
	user := &GoogleUserInfo{Email: "", Name: "No Email User"}

	token, _ := svc.GenerateToken(user)
	_, err := svc.ValidateToken(token)
	if err == nil {
		t.Error("expected error for token without email")
	}
}
