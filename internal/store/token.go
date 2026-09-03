package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// TokenSource says where a token came from.
type TokenSource string

const (
	TokenFromFile        TokenSource = "file"
	TokenFromEnvironment TokenSource = "environment"
)

// ErrNoToken means neither the token file nor DISCORD_TOKEN is set.
var ErrNoToken = errors.New("no user token found")

// LoadToken returns the token and its source: the token file first, then
// DISCORD_TOKEN.
func LoadToken(p Paths, getenv func(string) string) (string, TokenSource, error) {
	b, err := os.ReadFile(p.TokenFile())
	if err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, TokenFromFile, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("reading %s: %w", p.TokenFile(), err)
	}
	if t := strings.TrimSpace(getenv("DISCORD_TOKEN")); t != "" {
		return t, TokenFromEnvironment, nil
	}
	return "", "", ErrNoToken
}

// SaveToken writes the token file with mode 0600, creating the directory.
func SaveToken(p Paths, token string) error {
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		return err
	}
	return WriteFileAtomic(p.TokenFile(), []byte(token+"\n"), 0o600)
}
