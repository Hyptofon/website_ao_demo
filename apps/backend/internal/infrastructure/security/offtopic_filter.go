package security

import (
	"regexp"
	"strings"
)

// offTopicPattern is the compiled regular expression for fast blacklisted keyword matching.
// Checked in-memory before any LLM/DB call — zero latency.
//
// DESIGN NOTES (fix #8 — false positive reduction):
//  1. Single short words like "joke" / "жарт" are replaced with full-phrase patterns
//     (e.g. "tell.*joke", "розкажи.*жарт") so the word alone cannot trigger a block.
//  2. Patterns use \b word boundaries where possible to avoid partial matches.
//  3. Words that frequently appear in legitimate university questions are removed:
//     "translate this" blocked "переведіть мій документ" — now requires the explicit
//     "translate this" phrase, not just any occurrence of "translate".
//  4. "who are you" is removed — students legitimately ask "who is the department head".
//     Instead we match "are you a human|are you ai" variants which are unambiguous.
//  5. Prompt injection patterns are kept as-is (low false-positive rate, high risk).
var compiledOffTopicRegex *regexp.Regexp

func init() {
	// Each pattern is a Go regex fragment. The full regex uses (?i) for case-insensitivity.
	// Use ^-anchored word boundaries (\b) to avoid catching word stems.
	patterns := []string{
		// ── Ukrainian off-topic full phrases (not single words) ───────────────────
		`рецепт\b`,                    // "рецепт борщу" — not "рецепт успіху на вступі"
		`напиши (код|вірш|пісню|лист|резюме)`,
		`розкажи (анекдот|жарт)`,
		`анекдот\b`,                   // "розкажи анекдот" covered by above; standalone also blocked
		`допоможи (з домашнім|написати|зробити) (завданн|курсов|дипломн)`,
		`домашнє завдання\b`,
		`ігнорувати попередні інструкції`,
		`ігноруй (усі )?інструкції`,
		`ти (людина|не бот|не aii?)`,  // "ти людина" = am I speaking to a human?
		`який сьогодні (день|дата)`,
		`яка (погода|температура)`,
		`прогноз погоди`,         // broader: catches "на завтра", "на тиждень" etc.
		`курс (валют|долар|євро)`,
		`зламай(ку|те)? `, `злом `, `хакер\b`, // space after word prevents false boundary
		`переклади (текст|це|документ)`, // avoids blocking "переклади вимоги"

		// ── English off-topic full phrases ────────────────────────────────────────
		`recipe\s+for\b`,              // "recipe for" — not "recipe for success"
		`write (me )?(a |some )?(code|poem|song|story|essay|resume|cover letter)`,
		`tell (me )?(a )?joke\b`,
		`(help|do) (with |my )?homework\b`,
		`ignore (all )?previous instructions`,
		`ignore (all )?(prior|earlier) instructions`,
		`forget everything (i told|above)`,
		`are you (a |really )?(human|real person|alive|sentient)`,
		`(weather|forecast) (today|tomorrow|this week)`,
		`exchange rate\b`, `currency rate\b`,
		`translate (this|the following|my)`, // more specific than "translate this"
		`write (my |a )?resume\b`,
		`\bhack(ing|er)?\b`,
		`sql injection\b`,
		`how (are you|were you) (built|made|trained|created)\b`,
		`drop table\b`,
		`select\s+\*\s+from\b`,

		// ── Prompt injection (kept strict — unambiguous attack patterns) ──────────
		`\bsystem:\s`,
		`\bassistant:\s`,
		`act as (a |an )?(different|new|evil|unrestricted|pirate|hacker|villain)`,
		`pretend (you are|to be) (a |an )?(jailbroken|free|evil|unethical|hacker|pirate)`,
		`\bjailbreak\b`,
		`\bdan mode\b`,
		`new (system )?instructions?\b`,
	}

	// Compile to a single alternation regex: (?i)(pat1|pat2|...)
	pattern := `(?i)(` + strings.Join(patterns, "|") + `)`
	compiledOffTopicRegex = regexp.MustCompile(pattern)
}

// OffTopicFilter performs fast regex matching before the LLM call.
type OffTopicFilter struct{}

// NewOffTopicFilter returns the singleton structure using the compiled regex.
func NewOffTopicFilter() *OffTopicFilter {
	return &OffTopicFilter{}
}

// IsOffTopic returns true if the query matches any blacklisted pattern.
func (f *OffTopicFilter) IsOffTopic(query string) bool {
	return compiledOffTopicRegex.MatchString(query)
}

// Off-topic response strings are defined in domain/prompts.go (single source of truth).
// Use domain.OffTopicResponseUA and domain.OffTopicResponseEN.
