package security

import "strings"

// SanitizeInput escapes HTML special characters in user input to prevent XSS.
// TZ §3.5 / §6.1: "Санітизація HTML/JS (XSS prevention)"
//
// This is applied at the application layer before any processing,
// ensuring no HTML/JS injection can reach the LLM prompt or stored data.
func SanitizeInput(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
		"`", "&#96;",
	)
	return r.Replace(s)
}

// DetectXSSPayload returns true if the input contains common XSS attack patterns.
// This is a fast pre-check that rejects obviously malicious input.
func DetectXSSPayload(s string) bool {
	lower := strings.ToLower(s)
	patterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"onmouseover=",
		"onfocus=",
		"<iframe",
		"<object",
		"<embed",
		"<svg/onload",
		"eval(",
		"document.cookie",
		"document.location",
		"window.location",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
