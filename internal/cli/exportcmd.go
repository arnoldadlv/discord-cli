package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// exportResultJSON is the JSON for one exported channel, thread, or DM.
type exportResultJSON struct {
	Guild        namedJSON `json:"guild"`
	Channel      namedJSON `json:"channel"`
	Path         string    `json:"path"`
	Status       string    `json:"status"`
	MessageCount int       `json:"message_count"`
	NewMessages  int       `json:"new_messages"`
	Error        string    `json:"error,omitempty"`
}

// exportRunner builds the export runner for this run.
func (a *app) exportRunner(full bool) (*export.Runner, error) {
	c, _, err := a.client()
	if err != nil {
		return nil, err
	}
	if a.metaStore == nil {
		a.metaStore = export.NewMetaStore(a.env.Now)
	}
	return &export.Runner{
		Client:    c,
		Locations: a.paths().ReadLocations(),
		Meta:      a.metaStore,
		Full:      full,
	}, nil
}

// channelTarget says where a channel's or thread's export goes.
func (a *app) channelTarget(g discord.Guild, ch discord.Channel, parent *discord.Channel) export.Target {
	guildDir := filepath.Join(a.paths().ExportsDir(), export.GuildDirName(g.Name))
	dir := guildDir
	if discord.IsThread(ch.Type) && parent != nil {
		dir = filepath.Join(guildDir, "threads", export.GuildDirName(parent.Name))
	}
	return export.Target{
		Guild:   export.Guild{ID: g.ID, Name: g.Name},
		Channel: export.Channel{ID: ch.ID, Name: ch.Name, Type: ch.Type},
		Dir:     dir,
		MetaDir: guildDir,
	}
}

// exportOne runs one export and reports it.
func (a *app) exportOne(ctx context.Context, runner *export.Runner, t export.Target, progressName string) (export.Result, error) {
	a.setupExportProgress(runner, map[string]string{t.Channel.ID: progressName})
	res, err := runner.Run(ctx, t)
	if a.env.StderrIsTerminal {
		fmt.Fprint(a.stderr(), "\r\033[K")
	}
	return res, err
}

func (a *app) printExportResult(g discord.Guild, ch discord.Channel, res export.Result) error {
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), exportResultJSON{
			Guild:        namedJSON{ID: g.ID, Name: g.Name},
			Channel:      namedJSON{ID: ch.ID, Name: ch.Name, Type: intPtr(ch.Type)},
			Path:         res.Path,
			Status:       string(res.Status),
			MessageCount: res.MessageCount,
			NewMessages:  res.NewMessages,
		})
	}
	fmt.Fprintln(a.stdout(), a.exportLine(ch.Name, res))
	return nil
}

func (a *app) exportLine(name string, res export.Result) string {
	switch res.Status {
	case export.StatusUpToDate:
		return fmt.Sprintf("#%s: up to date (%d messages)  %s", name, res.MessageCount, a.out.Dim(res.Path))
	case export.StatusFailed:
		return fmt.Sprintf("#%s: %s", name, a.out.Red("failed"))
	}
	return fmt.Sprintf("#%s: %d messages (%d new)  %s", name, res.MessageCount, res.NewMessages, a.out.Dim(res.Path))
}

func (a *app) channelExport(cmd *cobra.Command, guildFlag, channelInput string, withThreads, full bool) error {
	ctx := cmd.Context()
	input, err := a.guildArg(guildFlag)
	if err != nil {
		return err
	}
	g, err := a.resolveGuild(ctx, input)
	if err != nil {
		return err
	}
	ch, err := a.resolveChannel(ctx, g.ID, channelInput, withThreads)
	if err != nil {
		return err
	}
	var parent *discord.Channel
	if discord.IsThread(ch.Type) {
		chs, err := a.channels(ctx, g.ID)
		if err != nil {
			return err
		}
		for i := range chs {
			if chs[i].ID == ch.ParentID {
				parent = &chs[i]
			}
		}
		if parent == nil {
			parent = &discord.Channel{ID: ch.ParentID, Name: ch.ParentID}
		}
	}
	runner, err := a.exportRunner(full)
	if err != nil {
		return err
	}
	res, err := a.exportOne(ctx, runner, a.channelTarget(g, ch, parent), ch.Name)
	if err != nil {
		return a.apiError(err)
	}
	if res.Status == export.StatusExported {
		a.updateIndex()
	}
	return a.printExportResult(g, ch, res)
}

// dmTarget says where a DM's export goes: the dm directory beside the
// guild directories, with the tool's own guild values.
func (a *app) dmTarget(d discord.DMChannel) export.Target {
	dir := filepath.Join(a.paths().ExportsDir(), export.DMDirName)
	return export.Target{
		Guild:   export.Guild{ID: export.DMGuildID, Name: export.DMGuildName},
		Channel: export.Channel{ID: d.ID, Name: d.DisplayName(), Type: d.Type},
		Dir:     dir,
		MetaDir: dir,
	}
}

func (a *app) dmExport(cmd *cobra.Command, input string, full bool) error {
	ctx := cmd.Context()
	d, err := a.resolveDM(ctx, input)
	if err != nil {
		return err
	}
	runner, err := a.exportRunner(full)
	if err != nil {
		return err
	}
	res, err := a.exportOne(ctx, runner, a.dmTarget(d), d.DisplayName())
	if err != nil {
		return a.apiError(err)
	}
	if res.Status == export.StatusExported {
		a.updateIndex()
	}
	g := discord.Guild{ID: export.DMGuildID, Name: export.DMGuildName}
	ch := discord.Channel{ID: d.ID, Name: d.DisplayName(), Type: d.Type}
	return a.printExportResult(g, ch, res)
}
