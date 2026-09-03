// Package cli is the command tree of the discord tool. Every command is
// reached through Run, which is the boundary the tests drive.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/store"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// RepoURL is where problems are reported.
const RepoURL = "https://github.com/arnoldadlv/discord-cli"

// globalFlags are registered on the root and read by every command.
type globalFlags struct {
	JSON    bool
	NoCache bool
	Timeout time.Duration
	NoColor bool
}

// app is the per-run state shared by the commands.
type app struct {
	env   Env
	flags globalFlags
	ran   bool // set once a command's RunE begins, to tell usage errors from runtime errors

	out term.Style // style for stdout
	err term.Style // style for stderr

	api         *discord.Client
	tokenSource store.TokenSource
	metaStore   *export.MetaStore
}

func (a *app) stdout() io.Writer { return a.env.Stdout }
func (a *app) stderr() io.Writer { return a.env.Stderr }

// notice prints a one-line notice on stderr.
func (a *app) notice(format string, args ...any) {
	fmt.Fprintf(a.env.Stderr, format+"\n", args...)
}

// Run executes the tool with the given environment and returns the exit code.
func Run(ctx context.Context, env Env) int {
	env.defaults()
	a := &app{env: env}
	root := a.rootCommand()
	root.SetArgs(env.Args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(env.Stderr, "discord: interrupted")
		return ExitUnexpected
	}
	fmt.Fprint(env.Stderr, humanError(err))
	return exitCode(err, a.ran)
}

// shortHelp is what a bare `discord` prints: concise, with examples.
func (a *app) shortHelp() string {
	h := a.out.Bold
	return `discord reads your own Discord account from the terminal: guilds, channels,
DMs, live search, and local exports you can search without limits.

` + h("Usage:") + `
  discord <noun> <verb> [flags]

` + h("Nouns:") + `
  guild     list, show, search, export
  channel   list, read, export
  dm        list, read, search, export
  export    list, search
  auth      set, status
  config    set, get
  cache     status, rebuild, clear

` + h("Examples:") + `
  discord auth set
  discord config set default-guild "My Guild"
  discord channel read general --limit 10
  discord guild search "access control" --json

Run 'discord --help' for every flag, or 'discord help <noun> [<verb>]'.
Report problems at ` + RepoURL + `
`
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "discord",
		Short: "Read, search, and export your own Discord account from the terminal",
		Long: `discord reads your own Discord account from the terminal: guilds, channels,
DMs, live search, and local exports you can search without limits.

Commands are noun then verb: discord <noun> <verb> [flags]. Every command
accepts --json for stable machine output; progress and errors go to stderr.

Exit codes: 0 success, 1 unexpected error, 2 usage error, 3 authentication
failed, 4 guild, channel, or DM not found, 5 no exports found, 6 rate limit
exhausted.

Report problems at ` + RepoURL,
		Example: `  discord auth set
  discord config set default-guild "My Guild"
  discord guild list
  discord channel read general --limit 10
  discord guild search "access control" --json
  discord guild export --threads
  discord export search "policy" --all --author kyle`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(a.env.Version),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.ran = true
			_, err := io.WriteString(cmd.OutOrStdout(), a.shortHelp())
			return err
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			a.ran = true
			a.out = term.Style{Enabled: term.ColorEnabled(a.env.StdoutIsTerminal, a.env.Getenv, a.flags.NoColor)}
			a.err = term.Style{Enabled: term.ColorEnabled(a.env.StderrIsTerminal, a.env.Getenv, a.flags.NoColor)}
			return nil
		},
	}
	root.SetIn(a.env.Stdin)
	root.SetOut(a.env.Stdout)
	root.SetErr(a.env.Stderr)
	root.SetVersionTemplate(`{{.Version}}` + "\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return UsageError("%s", err.Error()).WithHint("Run '%s --help' for usage.", cmd.CommandPath())
	})
	root.CompletionOptions.DisableDefaultCmd = true

	pf := root.PersistentFlags()
	pf.BoolVar(&a.flags.JSON, "json", false, "emit JSON on stdout instead of human output")
	pf.BoolVar(&a.flags.NoCache, "no-cache", false, "bypass the lookup cache of guilds, channels, and DMs")
	pf.DurationVar(&a.flags.Timeout, "timeout", 30*time.Second, "per-request timeout for Discord (e.g. 45s, 2m)")
	pf.BoolVar(&a.flags.NoColor, "no-color", false, "disable colour even on a terminal")

	for _, n := range a.nouns() {
		root.AddCommand(n)
	}
	return root
}

// noun builds a command that groups verbs; on its own it prints help, and an
// unknown verb gets a suggestion.
func (a *app) noun(use, short string) *cobra.Command {
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return unknownVerb(cmd, args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	return c
}

// exactOnePositional validates the single positional every command with a
// primary object takes, with a usage error naming it.
func exactOnePositional(name string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		switch len(args) {
		case 1:
			return nil
		case 0:
			return UsageError("missing the %s argument", name).WithHint("Usage: %s", cmd.UseLine())
		default:
			return UsageError("too many arguments: %s", strings.Join(args, " ")).WithHint("Usage: %s", cmd.UseLine())
		}
	}
}

func unknownVerb(cmd *cobra.Command, verb string) error {
	msg := fmt.Sprintf("unknown command %q for %q", verb, cmd.CommandPath())
	if s := cmd.SuggestionsFor(verb); len(s) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t")
	}
	return UsageError("%s", msg).WithHint("Run '%s --help' for the verbs it accepts.", cmd.CommandPath())
}

func (a *app) nouns() []*cobra.Command {
	with := func(n *cobra.Command, verbs ...*cobra.Command) *cobra.Command {
		for _, v := range verbs {
			n.AddCommand(v)
		}
		return n
	}
	return []*cobra.Command{
		with(a.noun("guild", "List, show, search, and export guilds"), a.guildCommands()...),
		with(a.noun("channel", "List, read, and export channels and threads"), a.channelCommands()...),
		with(a.noun("dm", "List, read, search, and export direct messages"), a.dmCommands()...),
		with(a.noun("export", "List and search the exports on disk"), a.exportListCommand(), a.exportSearchCommand()),
		with(a.noun("auth", "Store and check the user token"), a.authCommands()...),
		with(a.noun("config", "Set and get configuration such as the default guild"), a.configCommands()...),
		a.noun("cache", "Inspect, rebuild, and clear the lookup cache and search index"),
	}
}

func versionString(version string) string {
	s := "discord version " + version
	if bi, ok := debug.ReadBuildInfo(); ok {
		if version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			s = "discord version " + bi.Main.Version
		}
		s += "\n" + bi.GoVersion
		var rev, modified string
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				rev = kv.Value
			case "vcs.modified":
				modified = kv.Value
			}
		}
		if rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			s += " " + rev
			if modified == "true" {
				s += " (modified)"
			}
		}
	}
	return s
}
