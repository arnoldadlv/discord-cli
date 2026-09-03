package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/store"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

func (a *app) authCommands() []*cobra.Command {
	set := &cobra.Command{
		Use:   "set",
		Short: "Store your user token in a file only you can read",
		Long: `Store your user token in the config directory with mode 0600.

When stdin is a terminal you are prompted with echo off. When stdin is piped
the token is read from it, so the token never lands in shell history or a
process listing:

  printf '%s' "$TOKEN" | discord auth set

This tool works only with the token of your own account, never a bot token.
Automating a user account is against Discord's terms of service; use it on
your own account and at your own risk.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.authSet()
		},
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "Report where the token comes from and whether Discord accepts it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.authStatus(cmd)
		},
	}
	return []*cobra.Command{set, status}
}

func (a *app) readToken() (string, error) {
	if a.env.StdinIsTerminal {
		if a.env.ReadPassword == nil {
			return "", Errorf(ExitUnexpected, "cannot prompt for the token on this terminal").
				WithHint("Pipe it instead: printf '%%s' \"$TOKEN\" | discord auth set")
		}
		fmt.Fprint(a.env.Stderr, "Paste your user token (input is hidden): ")
		tok, err := a.env.ReadPassword()
		fmt.Fprintln(a.env.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(tok), nil
	}
	r := bufio.NewReader(a.env.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (a *app) authSet() error {
	tok, err := a.readToken()
	if err != nil {
		return err
	}
	if tok == "" {
		return UsageError("no token was given").WithHint("Paste the token at the prompt, or pipe it: printf '%%s' \"$TOKEN\" | discord auth set")
	}
	if strings.HasPrefix(strings.ToLower(tok), "bot ") {
		return Errorf(ExitAuth, "that looks like a bot token; discord-cli only works with a user token")
	}
	if err := store.SaveToken(a.paths(), tok); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}
	a.notice("Token stored in %s (mode 0600). Run 'discord auth status' to check it.", a.paths().TokenFile())
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), map[string]any{"stored": true, "path": a.paths().TokenFile()})
	}
	return nil
}

type authStatusJSON struct {
	Source   string `json:"source"`
	Path     string `json:"path,omitempty"`
	Username string `json:"username"`
	UserID   string `json:"user_id"`
	Valid    bool   `json:"valid"`
}

func (a *app) authStatus(cmd *cobra.Command) error {
	c, src, err := a.client()
	if err != nil {
		return err
	}
	u, err := c.CurrentUser(cmd.Context())
	if err != nil {
		return a.apiError(err)
	}
	out := authStatusJSON{Source: string(src), Username: u.Username, UserID: u.ID, Valid: true}
	if src == store.TokenFromFile {
		out.Path = a.paths().TokenFile()
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), out)
	}
	sourceText := "DISCORD_TOKEN in the environment"
	if src == store.TokenFromFile {
		sourceText = "token file " + out.Path
	}
	fmt.Fprintf(a.stdout(), "Token source: %s\n", sourceText)
	fmt.Fprintf(a.stdout(), "Account:      %s (%s)\n", a.out.Bold(u.Username), u.ID)
	fmt.Fprintf(a.stdout(), "Status:       %s\n", a.out.Green("accepted by Discord"))
	return nil
}
