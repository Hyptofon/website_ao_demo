package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ─── Resilient JSON Parsing ──────────────────────────────────────────────────
// Inspired by python_service-dev/utils/json_utils.py
// Handles malformed JSON from LLM responses: markdown wrappers, trailing commas,
// raw control characters inside string values.

var (
	trailingCommaArray  = regexp.MustCompile(`,\s*]`)
	trailingCommaObject = regexp.MustCompile(`,\s*}`)
)

// SanitizeGeminiJSON attempts to clean and fix JSON returned by Gemini.
// It performs a 3-pass repair:
//  1. Try to parse as-is
//  2. Strip markdown wrappers (```json), remove trailing commas
//  3. Brute-force: extract outer {…} or […], escape control chars
func SanitizeGeminiJSON(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty JSON input")
	}

	// Pass 1: try as-is
	if json.Valid([]byte(s)) {
		return s, nil
	}

	// Pass 2: strip markdown wrappers and trailing commas
	cleaned := stripMarkdownWrapper(s)
	cleaned = removeTrailingCommas(cleaned)

	if json.Valid([]byte(cleaned)) {
		return cleaned, nil
	}

	// Pass 2.5: escape raw control chars inside strings
	escaped := EscapeControlCharsInStrings(cleaned)
	if json.Valid([]byte(escaped)) {
		return escaped, nil
	}

	// Pass 3: brute-force — extract outermost JSON object or array
	extracted := extractOuterJSON(escaped)
	if extracted != "" && json.Valid([]byte(extracted)) {
		return extracted, nil
	}

	return "", fmt.Errorf("unfixable JSON after all repair attempts, first 200 chars: %s", truncate(raw, 200))
}

// SafeJSONUnmarshal tries to unmarshal raw bytes, applying sanitization on failure.
func SafeJSONUnmarshal(data []byte, target any) error {
	// Try direct unmarshal first
	if err := json.Unmarshal(data, target); err == nil {
		return nil
	}

	// Try sanitized version
	fixed, fixErr := SanitizeGeminiJSON(string(data))
	if fixErr != nil {
		return fmt.Errorf("JSON unmarshal failed and repair failed: %w", fixErr)
	}

	return json.Unmarshal([]byte(fixed), target)
}

// EscapeControlCharsInStrings walks the text char-by-char, tracking whether
// we are inside a JSON string value, and escapes only raw control characters
// (\n, \r, \t) found inside strings. Structural newlines outside strings
// are left intact.
//
// Inspired by python_service-dev/infrastructure/ai/gemini_service.py
// _escape_control_chars_in_strings()
func EscapeControlCharsInStrings(text string) string {
	var result strings.Builder
	result.Grow(len(text))

	inString := false
	escapeNext := false

	for _, ch := range text {
		if escapeNext {
			result.WriteRune(ch)
			escapeNext = false
			continue
		}

		if ch == '\\' && inString {
			escapeNext = true
			result.WriteRune(ch)
			continue
		}

		if ch == '"' {
			inString = !inString
			result.WriteRune(ch)
			continue
		}

		if inString {
			switch ch {
			case '\n':
				result.WriteString(`\n`)
				continue
			case '\r':
				result.WriteString(`\r`)
				continue
			case '\t':
				result.WriteString(`\t`)
				continue
			}
		}

		result.WriteRune(ch)
	}

	return result.String()
}

// stripMarkdownWrapper removes ```json ... ``` or ``` ... ``` wrappers
// commonly added by LLMs around JSON output.
func stripMarkdownWrapper(s string) string {
	s = strings.TrimSpace(s)

	// Remove leading ```json or ```
	if strings.HasPrefix(s, "```json") {
		s = s[7:]
	} else if strings.HasPrefix(s, "```") {
		s = s[3:]
	}

	// Remove trailing ```
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}

	return strings.TrimSpace(s)
}

// removeTrailingCommas removes trailing commas before ] and }
// which are a common LLM JSON error.
func removeTrailingCommas(s string) string {
	s = trailingCommaArray.ReplaceAllString(s, "]")
	s = trailingCommaObject.ReplaceAllString(s, "}")
	return s
}

// extractOuterJSON finds the first { and last } (or [ and ]) and extracts
// what's between them, discarding any surrounding prose.
func extractOuterJSON(s string) string {
	// Try object first
	firstBrace := strings.Index(s, "{")
	lastBrace := strings.LastIndex(s, "}")
	if firstBrace >= 0 && lastBrace > firstBrace {
		candidate := s[firstBrace : lastBrace+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	// Try array
	firstBracket := strings.Index(s, "[")
	lastBracket := strings.LastIndex(s, "]")
	if firstBracket >= 0 && lastBracket > firstBracket {
		candidate := s[firstBracket : lastBracket+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
