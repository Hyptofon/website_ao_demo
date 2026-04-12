package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── Google OAuth 2.0 (Google Identity Services) ────────────────────────────
// Uses the token endpoint and userinfo endpoint directly — no deprecated Google+ API.
// Reference: https://developers.google.com/identity/protocols/oauth2/web-server

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// OAuthConfig holds Google OAuth 2.0 credentials.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// GoogleUserInfo represents the user info returned by Google's userinfo endpoint.
type GoogleUserInfo struct {
	Sub           string `json:"sub"`            // Unique Google ID
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// tokenResponse is the JSON structure from Google's token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// OAuthService handles the Google OAuth 2.0 authorization code flow.
type OAuthService struct {
	config     OAuthConfig
	httpClient *http.Client
}

// NewOAuthService creates a new OAuth service with the given configuration.
func NewOAuthService(cfg OAuthConfig) *OAuthService {
	return &OAuthService{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetAuthURL returns the Google consent screen URL.
// The state parameter should be a CSRF token stored in session.
func (s *OAuthService) GetAuthURL(state string) string {
	params := url.Values{
		"client_id":     {s.config.ClientID},
		"redirect_uri":  {s.config.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"access_type":   {"offline"},
		"state":         {state},
		"prompt":        {"consent"},
	}
	return googleAuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for an access token.
func (s *OAuthService) ExchangeCode(ctx context.Context, code string) (string, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {s.config.ClientID},
		"client_secret": {s.config.ClientSecret},
		"redirect_uri":  {s.config.RedirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth: token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth: token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("oauth: parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("oauth: empty access token in response")
	}

	return tokenResp.AccessToken, nil
}

// GetUserInfo retrieves the Google user's profile using the access token.
func (s *OAuthService) GetUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", googleUserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("oauth: create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: get userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: userinfo failed (status %d): %s", resp.StatusCode, string(body))
	}

	var info GoogleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("oauth: parse userinfo: %w", err)
	}

	if info.Email == "" {
		return nil, fmt.Errorf("oauth: no email in userinfo response")
	}

	return &info, nil
}
