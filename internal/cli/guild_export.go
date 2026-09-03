package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/discord"
	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// exportSummaryJSON is the JSON for a guild (or DM directory) export run.
type exportSummaryJSON struct {
	Guild         namedJSON          `json:"guild"`
	Exported      int                `json:"exported"`
	UpToDate      int                `json:"up_to_date"`
	Failed        int                `json:"failed"`
	TotalMessages int                `json:"total_messages"`
	NewMessages   int                `json:"new_messages"`
	Channels      []exportResultJSON `json:"channels"`
}

// exportJob is one channel or thread to export.
type exportJob struct {
	channel discord.Channel
	parent  *discord.Channel
}

func (a *app) guildExport(cmd *cobra.Command, guildFlag string, withThreads, full bool, concurrency int) error {
	ctx := cmd.Context()
	if concurrency < 1 {
		return UsageError("--concurrency must be at least 1")
	}
	input, err := a.guildArg(guildFlag)
	if err != nil {
		return err
	}
	g, err := a.resolveGuild(ctx, input)
	if err != nil {
		return err
	}
	chs, err := a.channels(ctx, g.ID)
	if err != nil {
		return err
	}
	var jobs []exportJob
	for _, ch := range chs {
		if discord.IsMessageChannel(ch.Type) {
			jobs = append(jobs, exportJob{channel: ch})
		}
	}
	if withThreads {
		threads, err := a.threadsByParent(ctx, chs)
		if err != nil {
			return err
		}
		for i := range chs {
			for _, t := range threads[chs[i].ID] {
				parent := chs[i]
				jobs = append(jobs, exportJob{channel: t, parent: &parent})
			}
		}
	}
	runner, err := a.exportRunner(full)
	if err != nil {
		return err
	}
	if a.env.StderrIsTerminal {
		a.notice("Exporting %d channels from %s, %d at a time", len(jobs), g.Name, concurrency)
	}
	names := map[string]string{}
	for _, j := range jobs {
		names[j.channel.ID] = j.channel.Name
	}
	a.setupExportProgress(runner, names)

	results := make([]exportResultJSON, len(jobs))
	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, concurrency)
		outM sync.Mutex
	)
	for i, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, job exportJob) {
			defer wg.Done()
			defer func() { <-sem }()
			t := a.channelTarget(g, job.channel, job.parent)
			res, err := runner.Run(ctx, t)
			outM.Lock()
			defer outM.Unlock()
			r := exportResultJSON{
				Guild:        namedJSON{ID: g.ID, Name: g.Name},
				Channel:      namedJSON{ID: job.channel.ID, Name: job.channel.Name, Type: intPtr(job.channel.Type)},
				Path:         res.Path,
				Status:       string(res.Status),
				MessageCount: res.MessageCount,
				NewMessages:  res.NewMessages,
			}
			if err != nil {
				r.Status = string(export.StatusFailed)
				r.Error = a.apiError(err).Error()
				if !errors.Is(err, context.Canceled) {
					a.notice("#%s: %s", job.channel.Name, firstLine(r.Error))
				}
			} else if a.env.StderrIsTerminal {
				fmt.Fprintf(a.stderr(), "\r\033[K  #%s: %s\n", job.channel.Name, r.Status)
			}
			results[i] = r
		}(i, job)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	for _, r := range results {
		if export.Status(r.Status) == export.StatusExported {
			a.updateIndex()
			break
		}
	}
	summary := exportSummaryJSON{Guild: namedJSON{ID: g.ID, Name: g.Name}, Channels: results}
	for _, r := range results {
		switch export.Status(r.Status) {
		case export.StatusExported:
			summary.Exported++
		case export.StatusUpToDate:
			summary.UpToDate++
		default:
			summary.Failed++
		}
		summary.TotalMessages += r.MessageCount
		summary.NewMessages += r.NewMessages
	}
	if a.flags.JSON {
		if err := term.WriteJSON(a.stdout(), summary); err != nil {
			return err
		}
	} else {
		w := a.stdout()
		for _, r := range results {
			fmt.Fprintln(w, a.exportLine(r.Channel.Name, export.Result{Path: r.Path, Status: export.Status(r.Status), MessageCount: r.MessageCount, NewMessages: r.NewMessages}))
		}
		fmt.Fprintf(w, "\n%s: %d exported, %d up to date, %d failed; %d messages on disk (%d new)\n",
			a.out.Bold(g.Name), summary.Exported, summary.UpToDate, summary.Failed, summary.TotalMessages, summary.NewMessages)
	}
	if summary.Failed > 0 {
		return Errorf(ExitUnexpected, "%d of %d exports failed; run again to retry them", summary.Failed, len(results))
	}
	return nil
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

// setupExportProgress makes the runner report page progress on a terminal.
// names maps channel ids to names; a single export passes just its own.
func (a *app) setupExportProgress(runner *export.Runner, names map[string]string) {
	if !a.env.StderrIsTerminal {
		return
	}
	var mu sync.Mutex
	runner.Progress = func(channelID string, fetched int) {
		mu.Lock()
		defer mu.Unlock()
		name := names[channelID]
		if name == "" {
			name = channelID
		}
		fmt.Fprintf(a.stderr(), "\r  #%s: %d messages fetched...", name, fetched)
	}
}
