package search

import (
	"regexp"
	"strings"
	"unicode"
)

// fts5SpecialChars are characters that carry special meaning in FTS5 query
// syntax and must be stripped or escaped before building a MATCH expression.
var fts5SpecialChars = regexp.MustCompile(`[*"():^{}+\-|!]`)

// collapseSpaces normalizes runs of whitespace down to a single space.
var collapseSpaces = regexp.MustCompile(`\s{2,}`)

// SanitizeQuery cleans user-supplied search text so it is safe for use in
// an FTS5 MATCH expression.
//
// Steps:
//  1. Trim leading/trailing whitespace
//  2. Fold to lowercase for case-insensitive matching
//  3. Strip FTS5 special characters to prevent query-syntax injection
//  4. Collapse multiple spaces into one
//  5. Split into tokens, append '*' for prefix matching
//
// Returns the cleaned FTS5 query string and a boolean indicating whether
// the result is non-empty (i.e. there is something to search for).
func SanitizeQuery(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	s = strings.ToLower(s)
	s = fts5SpecialChars.ReplaceAllString(s, " ")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = collapseSpaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	tokens := strings.Fields(s)
	for i, t := range tokens {
		tokens[i] = t + "*"
	}
	return strings.Join(tokens, " "), true
}

// SanitizeTagName normalizes a tag name for exact-match filtering.
// Returns the lowercased, trimmed name and whether it is non-empty.
func SanitizeTagName(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	return strings.ToLower(s), true
}
