package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/arnoldadlv/discord-cli/internal/export"
	"github.com/arnoldadlv/discord-cli/internal/search"
	"github.com/arnoldadlv/discord-cli/internal/store"
	"github.com/arnoldadlv/discord-cli/internal/term"
)

// allExports lists every export on disk once per run.
func (a *app) allExports() []export.Item {
	if a.exportsOnDisk == nil {
		a.exportsOnDisk = export.Inventory(a.paths().ReadLocations())
		if a.exportsOnDisk == nil {
			a.exportsOnDisk = []export.Item{}
		}
	}
	return a.exportsOnDisk
}

// updateIndex re-indexes changed exports after an export run. Failures are
// reported, never fatal: the JSON on disk is the truth.
func (a *app) updateIndex() {
	a.exportsOnDisk = nil
	items := a.allExports()
	ix, err := search.Open(a.paths().IndexFile())
	if err != nil {
		a.notice("Search index not updated: %v", err)
		return
	}
	defer ix.Close()
	if _, err := ix.Update(items, nil); err != nil {
		a.notice("Search index not updated: %v (run 'discord cache rebuild')", err)
	}
}

// runLocalSearch uses the index when every export on disk is indexed at
// its current size and modification time, and scans otherwise.
func (a *app) runLocalSearch(cmd *cobra.Command, items []export.Item, q search.Query) ([]search.Result, error) {
	all := a.allExports()
	indexPath := a.paths().IndexFile()
	if search.Exists(indexPath) {
		ix, err := search.Open(indexPath)
		if err == nil {
			defer ix.Close()
			stale, err := ix.Stale(all)
			if err == nil && stale == 0 {
				paths := make([]string, len(items))
				for i, it := range items {
					paths[i] = it.Path
				}
				res, err := ix.Search(paths, q)
				if err == nil {
					return res, nil
				}
				a.notice("Search index failed (%v); scanning exports instead. Run 'discord cache rebuild' to repair it.", err)
			} else if err == nil {
				a.notice("Search index is out of date for %s; scanning exports instead. Run 'discord cache rebuild' to make searches fast.", plural(stale, "file"))
			} else {
				a.notice("Search index unreadable (%v); scanning exports instead. Run 'discord cache rebuild' to repair it.", err)
			}
		} else {
			a.notice("Search index unreadable (%v); scanning exports instead. Run 'discord cache rebuild' to repair it.", err)
		}
	} else {
		a.notice("No search index yet; scanning exports instead. Run 'discord cache rebuild' to make searches fast.")
	}
	return search.Scan(items, q)
}

type indexStatusJSON struct {
	Present      bool   `json:"present"`
	Path         string `json:"path"`
	SizeBytes    int64  `json:"size_bytes"`
	MessageCount int    `json:"message_count"`
	FilesIndexed int    `json:"files_indexed"`
	FilesOnDisk  int    `json:"files_on_disk"`
	FilesStale   int    `json:"files_stale"`
}

type lookupStatusJSON struct {
	Name      string `json:"name"`
	Age       string `json:"age"`
	FetchedAt string `json:"fetched_at"`
	Fresh     bool   `json:"fresh"`
}

type cacheStatusJSON struct {
	Index  indexStatusJSON    `json:"index"`
	Lookup []lookupStatusJSON `json:"lookup"`
}

func (a *app) cacheCommands() []*cobra.Command {
	status := &cobra.Command{
		Use:   "status",
		Short: "Show what is cached and indexed, and how old it is",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cacheStatus()
		},
	}
	rebuild := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the search index from the exports on disk",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cacheRebuild()
		},
	}
	clear := &cobra.Command{
		Use:   "clear",
		Short: "Delete the search index and the lookup cache, nothing else",
		Long: `Delete the search index and the lookup cache. Exports, configuration, and
the token are never touched; the index is rebuilt from the exports on the
next 'discord cache rebuild' or export.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cacheClear()
		},
	}
	return []*cobra.Command{status, rebuild, clear}
}

func (a *app) cacheStatus() error {
	p := a.paths()
	st := cacheStatusJSON{Index: indexStatusJSON{Path: p.IndexFile()}, Lookup: []lookupStatusJSON{}}
	items := a.allExports()
	st.Index.FilesOnDisk = len(items)
	if search.Exists(p.IndexFile()) {
		st.Index.Present = true
		if info, err := os.Stat(p.IndexFile()); err == nil {
			st.Index.SizeBytes = info.Size()
		}
		ix, err := search.Open(p.IndexFile())
		if err != nil {
			return fmt.Errorf("opening index: %w", err)
		}
		defer ix.Close()
		files, msgs, err := ix.Stats()
		if err != nil {
			return err
		}
		st.Index.FilesIndexed, st.Index.MessageCount = files, msgs
		if st.Index.FilesStale, err = ix.Stale(items); err != nil {
			return err
		}
	}
	entries, _ := os.ReadDir(p.LookupCacheDir())
	lc := a.lookupCache()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		age, ok := lc.Age(name)
		if !ok {
			continue
		}
		st.Lookup = append(st.Lookup, lookupStatusJSON{
			Name:      name,
			Age:       age.Round(time.Second).String(),
			FetchedAt: a.env.Now().Add(-age).UTC().Format(time.RFC3339),
			Fresh:     age < store.LookupTTL,
		})
	}
	sort.Slice(st.Lookup, func(i, j int) bool { return st.Lookup[i].Name < st.Lookup[j].Name })

	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), st)
	}
	w := a.stdout()
	fmt.Fprintln(w, a.out.Bold("Search index"))
	if !st.Index.Present {
		fmt.Fprintf(w, "  no index yet; %s on disk. Run 'discord cache rebuild' or any export to build it.\n", plural(st.Index.FilesOnDisk, "export"))
	} else {
		fmt.Fprintf(w, "  %s  %s\n", a.shortPath(st.Index.Path), a.out.Dim(humanBytes(st.Index.SizeBytes)))
		fmt.Fprintf(w, "  %s in %d of %s on disk", plural(st.Index.MessageCount, "message"), st.Index.FilesIndexed, plural(st.Index.FilesOnDisk, "export"))
		if st.Index.FilesStale > 0 {
			fmt.Fprintf(w, "; %s %s", a.out.Yellow(plural(st.Index.FilesStale, "file")), a.out.Yellow("out of date (searches will scan)"))
		} else {
			fmt.Fprintf(w, "; %s", a.out.Green("up to date"))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, a.out.Bold("Lookup cache"))
	if len(st.Lookup) == 0 {
		fmt.Fprintln(w, "  empty (guilds, channels, and DMs are cached for 24 hours after first use)")
	}
	for _, l := range st.Lookup {
		state := a.out.Green("fresh")
		if !l.Fresh {
			state = a.out.Yellow("stale")
		}
		fmt.Fprintf(w, "  %-24s %s old  %s\n", l.Name, l.Age, state)
	}
	return nil
}

func (a *app) cacheRebuild() error {
	p := a.paths()
	if err := search.Remove(p.IndexFile()); err != nil {
		return err
	}
	a.exportsOnDisk = nil
	items := a.allExports()
	ix, err := search.Open(p.IndexFile())
	if err != nil {
		return err
	}
	defer ix.Close()
	var progress func(string)
	if a.env.StderrIsTerminal {
		progress = func(path string) { fmt.Fprintf(a.stderr(), "\r\033[K  indexing %s", a.shortPath(path)) }
	}
	n, err := ix.Update(items, progress)
	if a.env.StderrIsTerminal {
		fmt.Fprint(a.stderr(), "\r\033[K")
	}
	if err != nil {
		return fmt.Errorf("rebuilding index: %w", err)
	}
	_, msgs, err := ix.Stats()
	if err != nil {
		return err
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), map[string]any{"files_indexed": n, "message_count": msgs, "path": p.IndexFile()})
	}
	fmt.Fprintf(a.stdout(), "Indexed %s, %s, into %s\n", plural(n, "file"), plural(msgs, "message"), a.shortPath(p.IndexFile()))
	return nil
}

func (a *app) cacheClear() error {
	p := a.paths()
	if err := search.Remove(p.IndexFile()); err != nil {
		return err
	}
	if err := a.lookupCache().Clear(); err != nil {
		return err
	}
	if a.flags.JSON {
		return term.WriteJSON(a.stdout(), map[string]any{"cleared": true, "index": p.IndexFile(), "lookup_cache": p.LookupCacheDir()})
	}
	a.notice("Removed the search index and lookup cache under %s. Exports, configuration, and the token were not touched.", a.shortPath(filepath.Dir(p.IndexFile())))
	return nil
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
