package utils

import (
	"embed"
	"strings"
)

// LoadPrompt reads an embedded prompt file and returns its contents.
// Trims whitespace from the result.
func LoadPrompt(fs embed.FS, path string) (string, error) {
	content, err := fs.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// MustLoadPrompt reads an embedded prompt file and panics on error.
// Use this for prompts that must exist at compile time.
func MustLoadPrompt(fs embed.FS, path string) string {
	content, err := LoadPrompt(fs, path)
	if err != nil {
		panic("failed to load prompt: " + err.Error())
	}
	return content
}
