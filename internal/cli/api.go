package cli

import (
	"context"
	"errors"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/store"
)

// paths resolves the run's directories from the environment.
func (a *app) paths() store.Paths {
	return store.PathsFromEnv(a.env.Getenv)
}

// token loads the user token, printing the one-line notice when it came from
// the environment.
func (a *app) token() (string, store.TokenSource, error) {
	tok, src, err := store.LoadToken(a.paths(), a.env.Getenv)
	if err != nil {
		if errors.Is(err, store.ErrNoToken) {
			return "", "", Errorf(ExitAuth, "no user token found").
				WithHint("Run 'discord auth set' to store one, or set DISCORD_TOKEN in the environment.")
		}
		return "", "", err
	}
	if src == store.TokenFromEnvironment {
		a.notice("Using DISCORD_TOKEN from the environment; run 'discord auth set' to store it in a file only you can read.")
	}
	return tok, src, nil
}

// client builds the Discord client for this run.
func (a *app) client() (*discord.Client, store.TokenSource, error) {
	if a.api != nil {
		return a.api, a.tokenSource, nil
	}
	tok, src, err := a.token()
	if err != nil {
		return nil, "", err
	}
	c := discord.New(a.env.APIBaseURL, tok, discord.LocalTimezone(a.env.Getenv), a.flags.Timeout, a.env.Sleep)
	if a.env.StderrIsTerminal {
		c.Notice = func(s string) { a.notice("%s", s) }
	}
	a.api = c
	a.tokenSource = src
	return c, src, nil
}

// apiError turns a client error into an ExitError with the right code.
func (a *app) apiError(err error) error {
	if err == nil {
		return nil
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, discord.ErrBotToken) {
		return Errorf(ExitAuth, "%s", err.Error()).WithHint("Store the token of your own account with 'discord auth set'.")
	}
	if errors.Is(err, discord.ErrRateLimitExhausted) {
		return Errorf(ExitRateLimited, "Discord kept rate limiting this request; gave up after %d attempts", discord.MaxAttempts).
			WithHint("Wait a minute before trying again, and lower --concurrency for exports.")
	}
	var te *discord.TimeoutError
	if errors.As(err, &te) {
		return Errorf(ExitUnexpected, "%s", te.Error()).WithHint("Raise the limit with --timeout, for example --timeout 2m.")
	}
	var se *discord.StatusError
	if errors.As(err, &se) {
		switch se.Status {
		case 401:
			return Errorf(ExitAuth, "Discord rejected the token (401 Unauthorized)").
				WithHint("Run 'discord auth set' with a fresh user token. Changing your Discord password invalidates old tokens.")
		case 403:
			if se.Path == "/users/@me" {
				return Errorf(ExitAuth, "Discord refused the token (403 Forbidden): %s", se.Message).
					WithHint("Run 'discord auth set' with a fresh user token.")
			}
			return Errorf(ExitNotFound, "the account cannot see this (403 Forbidden): %s", se.Message).
				WithHint("Check the name or id; the channel may be private to you.")
		case 404:
			return Errorf(ExitNotFound, "Discord has no such resource (404): %s", se.Message)
		}
		return Errorf(ExitUnexpected, "%s", se.Error())
	}
	return err
}
