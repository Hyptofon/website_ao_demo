package security

import (
	"strings"
	"unicode"
)

// offTopicPatterns is the keyword blacklist for Stage 1 filtering.
// Checked in-memory before any LLM/DB call — zero latency.
var offTopicPatterns = []string{
	// Ukrainian
	"рецепт", "напиши код", "напиши вірш", "напиши пісню",
	"розкажи анекдот", "анекдот", "жарт",
	"допоможи з домашнім", "домашнє завдання",
	"ігнорувати попередні інструкції", "ігноруй інструкції",
	"ти людина", "ти людина?", "хто ти", "хто ти?",
	"який сьогодні день", "яка погода",
	"переклади", "переведи текст",
	"напиши резюме", "напиши лист",
	"прогноз погоди", "курс валют",
	"зламай", "злом", "хакер",
	// English
	"recipe", "write code", "write a poem", "write a song",
	"tell me a joke", "tell a joke", "joke",
	"help with homework", "do my homework",
	"ignore previous instructions", "ignore all previous",
	"are you human", "who are you",
	"what day is it", "weather forecast",
	"translate this", "write my resume",
	"hack", "hacking", "sql injection",
	"how are you built", "what are you built with",
	"drop table", "select * from",
	// Prompt injection
	"system:", "assistant:", "forget everything",
	"new instructions", "act as", "pretend you are",
	"jailbreak", "dan mode",
}

// OffTopicFilter performs Stage 1 fast keyword matching before the LLM call.
type OffTopicFilter struct {
	patterns []string
}

// NewOffTopicFilter returns a configured filter with the default blacklist.
func NewOffTopicFilter() *OffTopicFilter {
	return &OffTopicFilter{patterns: offTopicPatterns}
}

// IsOffTopic returns true if the query matches any blacklisted pattern.
func (f *OffTopicFilter) IsOffTopic(query string) bool {
	lower := strings.Map(unicode.ToLower, query)
	for _, p := range f.patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// OffTopicResponseUA is the canned reply in Ukrainian.
const OffTopicResponseUA = "Вибачте, я можу відповідати виключно на питання про вступ та навчання на кафедрі. Спробуйте запитати про спеціальності, необхідні документи або умови вступу."

// OffTopicResponseEN is the canned reply in English.
const OffTopicResponseEN = "Sorry, I can only answer questions about admission and studies at the department. Try asking about specialties, required documents, or admission requirements."
