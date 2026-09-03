package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/resolve"
	"github.com/arnoldadlv/discord-cli/internal/store"
)

func (a *app) lookupCache() store.LookupCache {
	return store.LookupCache{Dir: a.paths().LookupCacheDir(), Now: a.env.Now, Bypass: a.flags.NoCache}
}

// guilds returns the account's guilds, from the lookup cache when fresh.
func (a *app) guilds(ctx context.Context) ([]discord.Guild, error) {
	var out []discord.Guild
	_, err := a.lookupCache().Get("guilds", func() (any, error) {
		c, _, err := a.client()
		if err != nil {
			return nil, err
		}
		gs, err := c.Guilds(ctx)
		if err != nil {
			return nil, a.apiError(err)
		}
		return gs, nil
	}, &out)
	return out, err
}

// channels returns a guild's channel list, from the lookup cache when fresh.
func (a *app) channels(ctx context.Context, guildID string) ([]discord.Channel, error) {
	var out []discord.Channel
	_, err := a.lookupCache().Get("channels-"+guildID, func() (any, error) {
		c, _, err := a.client()
		if err != nil {
			return nil, err
		}
		chs, err := c.Channels(ctx, guildID)
		if err != nil {
			return nil, a.apiError(err)
		}
		return chs, nil
	}, &out)
	return out, err
}

// addGuildFlag registers --guild on a command.
func addGuildFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVarP(dst, "guild", "g", "", "guild name or id (default: the configured default-guild)")
}

// guildArg picks the guild from --guild, else the configured default, else a
// usage error.
func (a *app) guildArg(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	cfg, err := store.LoadConfig(a.paths())
	if err != nil {
		return "", err
	}
	if cfg.DefaultGuild != "" {
		return cfg.DefaultGuild, nil
	}
	return "", UsageError("no guild given and no default guild configured").
		WithHint("Pass --guild <name or id>, or run 'discord config set default-guild <name>' once.")
}

// resolveGuild turns a name or id into a guild.
func (a *app) resolveGuild(ctx context.Context, input string) (discord.Guild, error) {
	gs, err := a.guilds(ctx)
	if err != nil {
		return discord.Guild{}, err
	}
	cands := make([]resolve.Candidate, len(gs))
	for i, g := range gs {
		cands[i] = resolve.Candidate{ID: g.ID, Name: g.Name}
	}
	m, err := resolve.Match("guild", input, cands)
	if err != nil {
		return discord.Guild{}, a.resolveError(err, "guild list")
	}
	for _, g := range gs {
		if g.ID == m.ID {
			return g, nil
		}
	}
	// A numeric id not in the list: ask Discord directly.
	c, _, err := a.client()
	if err != nil {
		return discord.Guild{}, err
	}
	g, err := c.Guild(ctx, m.ID)
	if err != nil {
		if discord.IsNotFound(err) {
			return discord.Guild{}, Errorf(ExitNotFound, "guild %q not found", input).WithHint("Run 'discord guild list' to see the guilds this account belongs to.")
		}
		return discord.Guild{}, a.apiError(err)
	}
	return *g, nil
}

// resolveError maps resolver errors to exit 4 with suggestions.
func (a *app) resolveError(err error, listCmd string) error {
	var nf *resolve.NotFoundError
	if errors.As(err, &nf) {
		e := Errorf(ExitNotFound, "%s", nf.Error())
		if len(nf.Suggestions) > 0 {
			e.Hint = "Did you mean: " + strings.Join(nf.Suggestions, ", ") + "?"
		} else {
			e.Hint = fmt.Sprintf("Run 'discord %s' to see what is available.", listCmd)
		}
		return e
	}
	var amb *resolve.AmbiguousError
	if errors.As(err, &amb) {
		return Errorf(ExitNotFound, "%s", amb.Error()).WithHint("Use the id to pick one.")
	}
	return err
}
