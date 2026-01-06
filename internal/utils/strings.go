package utils

import (
	"strings"
	"unicode/utf8"
)

// TruncateUTF8 safely truncates a string to maxRunes without cutting multi-byte characters.
func TruncateUTF8(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// SanitizeUTF8 removes invalid UTF-8 sequences and problematic characters
// that can cause issues with LLM APIs.
func SanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}
