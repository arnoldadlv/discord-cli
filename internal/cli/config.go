package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/store"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

func (a *app) configCommands() []*cobra.Command {
	set := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration key (default-guild)",
		Long: `Set a configuration key. Keys:

  default-guild   the guild commands act on when --guild is not given;
                  resolved once against your guild list and stored as typed`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configSet(cmd, args[0], args[1])
		},
	}
	get := &cobra.Command{
		Use:   "get [<key>]",
		Short: "Print the configuration, or one key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = args[0]
			}
			return a.configGet(key)
		},
	}
	return []*cobra.Command{set, get}
}

func (a *app) configSet(cmd *cobra.Command, key, value string) error {
	cfg, err := store.LoadConfig(a.paths())
	if err != nil {
		return err
	}
	switch key {
	case "default-guild":
		g, err := a.resolveGuild(cmd.Context(), value)
		if err != nil {
			return err
		}
		cfg.DefaultGuild = value
		if err := store.SaveConfig(a.paths(), cfg); err != nil {
			return err
		}
		a.notice("default-guild set to %q (%s, %s)", value, g.Name, g.ID)
		if a.flags.JSON {
			return term.WriteJSON(a.stdout(), map[string]any{"default-guild": value, "guild": namedJSON{ID: g.ID, Name: g.Name}})
		}
		return nil
	}
	return UsageError("unknown configuration key %q", key).WithHint("Keys: %s", strings.Join(store.ConfigKeys, ", "))
}

func (a *app) configGet(key string) error {
	cfg, err := store.LoadConfig(a.paths())
	if err != nil {
		return err
	}
	if key != "" {
		v, ok := cfg.Get(key)
		if !ok {
			return UsageError("unknown configuration key %q", key).WithHint("Keys: %s", strings.Join(store.ConfigKeys, ", "))
		}
		if a.flags.JSON {
			return term.WriteJSON(a.stdout(), map[string]string{key: v})
		}
		fmt.Fprintln(a.stdout(), v)
		return nil
	}
	all := map[string]string{}
	for _, k := range store.ConfigKeys {
		v, _ := cfg.Get(k)
		all[k] = v
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), all)
	}
	for _, k := range store.ConfigKeys {
		v := all[k]
		if v == "" {
			v = a.out.Dim("(unset)")
		}
		fmt.Fprintf(a.stdout(), "%s = %s\n", k, v)
	}
	fmt.Fprintf(a.stdout(), "%s\n", a.out.Dim("file: "+a.paths().ConfigFile()))
	return nil
}
