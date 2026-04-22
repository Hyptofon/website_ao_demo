package http

import "testing"

func TestIsEmailAllowed_ExactMatch(t *testing.T) {
	allowed := []string{"admin@test.com", "user@example.com"}
	if !isEmailAllowed("admin@test.com", allowed) {
		t.Error("Expected admin@test.com to be allowed")
	}
	if isEmailAllowed("hacker@evil.com", allowed) {
		t.Error("Expected hacker@evil.com to be rejected")
	}
}

func TestIsEmailAllowed_DomainMatch(t *testing.T) {
	allowed := []string{"@university.edu.ua"}
	if !isEmailAllowed("professor@university.edu.ua", allowed) {
		t.Error("Expected professor@university.edu.ua to be allowed")
	}
	if !isEmailAllowed("student@university.edu.ua", allowed) {
		t.Error("Expected student@university.edu.ua to be allowed")
	}
	if isEmailAllowed("user@gmail.com", allowed) {
		t.Error("Expected user@gmail.com to be rejected")
	}
}

func TestIsEmailAllowed_MixedRules(t *testing.T) {
	allowed := []string{"@university.edu.ua", "special@gmail.com"}
	if !isEmailAllowed("admin@university.edu.ua", allowed) {
		t.Error("Expected domain match to work")
	}
	if !isEmailAllowed("special@gmail.com", allowed) {
		t.Error("Expected exact match to work")
	}
	if isEmailAllowed("other@gmail.com", allowed) {
		t.Error("Expected non-matching email to be rejected")
	}
}

func TestIsEmailAllowed_EmptyWhitelist(t *testing.T) {
	if !isEmailAllowed("anyone@anywhere.com", nil) {
		t.Error("Empty whitelist should allow all")
	}
	if !isEmailAllowed("anyone@anywhere.com", []string{}) {
		t.Error("Empty whitelist should allow all")
	}
}

func TestIsEmailAllowed_CaseInsensitive(t *testing.T) {
	allowed := []string{"Admin@Test.Com"}
	if !isEmailAllowed("admin@test.com", allowed) {
		t.Error("Expected case-insensitive match")
	}
}
