package cli_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

// serveThreads registers the per-channel thread search endpoint for every
// fixture channel, paging one thread per response so tests observe offset.
func serveThreads(r *clitest.Runner) {
	for _, ch := range clitest.Channels() {
		id := ch["id"].(string)
		r.Fake.Handle("/channels/"+id+"/threads/search", func(req *http.Request) clitest.Response {
			active, archived := clitest.Threads(id)
			list := active
			if req.URL.Query().Get("archived") == "true" {
				list = archived
			}
			offset := 0
			if o := req.URL.Query().Get("offset"); o != "" {
				for _, c := range o {
					offset = offset*10 + int(c-'0')
				}
			}
			var page []map[string]any
			if offset < len(list) {
				page = list[offset : offset+1]
			}
			return clitest.Response{Status: 200, Body: map[string]any{
				"threads":       page,
				"has_more":      offset+1 < len(list),
				"total_results": len(list),
			}}
		})
	}
}

func channelRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := guildRunner(t)
	serveThreads(r)
	r.Run("config", "set", "default-guild", "cooey-coe")
	r.Fake.Reset()
	return r
}

func TestChannelListGroupsByCategoryAndFiltersTypes(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("channel", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	out := res.Stdout
	for _, want := range []string{"Text Channels", "🔮general", "📰news", "cmmc-general", "Support", "help-forum", "random", "2001", "2006"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, no := range []string{"Lounge", "2005"} {
		if strings.Contains(out, no) {
			t.Errorf("voice channel %q listed:\n%s", no, out)
		}
	}
	// Uncategorised first, then categories in position order.
	if !(strings.Index(out, "random") < strings.Index(out, "Text Channels") && strings.Index(out, "Text Channels") < strings.Index(out, "Support")) {
		t.Errorf("order wrong:\n%s", out)
	}
	if strings.Contains(out, "Voice") && strings.Index(out, "Voice") < strings.Index(out, "Support") && strings.Contains(out, "\nVoice") {
		t.Errorf("empty category should not be printed:\n%s", out)
	}
	if n := len(r.Fake.RequestsTo("/guilds/1001/channels")); n != 1 {
		t.Errorf("channel requests %d", n)
	}
}

func TestChannelListJSON(t *testing.T) {
	r := channelRunner(t)
	var out []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       int    `json:"type"`
		TypeLabel  string `json:"type_label"`
		Category   string `json:"category"`
		CategoryID string `json:"category_id"`
		Threads    []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		} `json:"threads"`
	}
	r.Run("channel", "list", "--json").JSON(t, &out)
	if len(out) != 5 {
		t.Fatalf("want 5 channels, got %d: %+v", len(out), out)
	}
	byID := map[string]int{}
	for i, c := range out {
		byID[c.ID] = i
	}
	news := out[byID["2002"]]
	if news.Name != "📰news" || news.Type != 5 || news.TypeLabel != "announcement" || news.Category != "Text Channels" || news.CategoryID != "2000" {
		t.Errorf("news: %+v", news)
	}
	if forum := out[byID["2006"]]; forum.TypeLabel != "forum" || forum.Category != "Support" {
		t.Errorf("forum: %+v", forum)
	}
	if rnd := out[byID["2007"]]; rnd.Category != "" {
		t.Errorf("random should have no category: %+v", rnd)
	}
	if len(r.Fake.RequestsTo("/channels/2001/threads/search")) != 0 {
		t.Errorf("threads fetched without --threads")
	}
}

func TestChannelListThreadsUsesPerChannelSearchOnly(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("channel", "list", "--threads")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"welcome thread", "old planning", "How do I scope?", "3001", "archived"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	if strings.Index(res.Stdout, "🔮general") > strings.Index(res.Stdout, "welcome thread") {
		t.Errorf("thread should follow its parent:\n%s", res.Stdout)
	}
	for _, req := range r.Fake.Requests() {
		if strings.Contains(req.Path, "/threads/active") || strings.Contains(req.Path, "/threads/archived") {
			t.Errorf("forbidden endpoint called: %s", req.Path)
		}
	}
	// Two passes per channel (archived=false then true), paging with offset.
	reqs := r.Fake.RequestsTo("/channels/2001/threads/search")
	var seq []string
	for _, q := range reqs {
		seq = append(seq, q.Query.Get("archived")+"@"+q.Query.Get("offset"))
		if q.Query.Get("sort_by") != "last_message_time" || q.Query.Get("sort_order") != "desc" || q.Query.Get("limit") != "25" {
			t.Errorf("query: %v", q.Query)
		}
	}
	want := "false@0 true@0"
	if got := strings.Join(seq, " "); got != want {
		t.Errorf("sequence %q, want %q", got, want)
	}
	// Every message channel is asked, voice and categories are not.
	if len(r.Fake.RequestsTo("/channels/2005/threads/search")) != 0 || len(r.Fake.RequestsTo("/channels/2000/threads/search")) != 0 {
		t.Errorf("thread search on non-message channel")
	}
	if len(r.Fake.RequestsTo("/channels/2007/threads/search")) == 0 {
		t.Errorf("random not asked for threads")
	}
}

func TestChannelListThreadsPagesWithOffset(t *testing.T) {
	r := channelRunner(t)
	// Make channel 2007 return three active threads one per page.
	r.Fake.Handle("/channels/2007/threads/search", func(req *http.Request) clitest.Response {
		q := req.URL.Query()
		if q.Get("archived") == "true" {
			return clitest.Response{Status: 200, Body: map[string]any{"threads": []any{}, "has_more": false}}
		}
		off := q.Get("offset")
		names := map[string]string{"0": "t-one", "1": "t-two", "2": "t-three"}
		return clitest.Response{Status: 200, Body: map[string]any{
			"threads":  []map[string]any{{"id": "39" + off, "name": names[off], "type": 11, "parent_id": "2007", "thread_metadata": map[string]any{"archived": false}}},
			"has_more": off != "2",
		}}
	})
	var out []struct {
		ID      string `json:"id"`
		Threads []struct {
			Name string `json:"name"`
		} `json:"threads"`
	}
	res := r.Run("channel", "list", "--threads", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &out)
	for _, c := range out {
		if c.ID == "2007" && len(c.Threads) != 3 {
			t.Errorf("random threads: %+v", c.Threads)
		}
	}
	offsets := []string{}
	for _, q := range r.Fake.RequestsTo("/channels/2007/threads/search") {
		if q.Query.Get("archived") == "false" {
			offsets = append(offsets, q.Query.Get("offset"))
		}
	}
	if strings.Join(offsets, ",") != "0,1,2" {
		t.Errorf("offsets %v", offsets)
	}
}

func TestChannelListThreadsToleratesForbiddenChannel(t *testing.T) {
	r := channelRunner(t)
	r.Fake.Queue("/channels/2003/threads/search", clitest.Response{Status: 403, Body: `{"message":"Missing Access","code":50001}`})
	res := r.Run("channel", "list", "--threads")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if n := len(r.Fake.RequestsTo("/channels/2003/threads/search")); n != 1 {
		t.Errorf("403 retried: %d requests", n)
	}
}

func TestChannelListIsCachedPerGuild(t *testing.T) {
	r := channelRunner(t)
	r.Run("channel", "list")
	r.Fake.Reset()
	res := r.Run("channel", "list")
	if res.ExitCode != 0 || len(r.Fake.Requests()) != 0 {
		t.Errorf("second list made %d requests: %s", len(r.Fake.Requests()), res.Stderr)
	}
	r.Run("channel", "list", "--no-cache")
	if n := len(r.Fake.RequestsTo("/guilds/1001/channels")); n != 1 {
		t.Errorf("--no-cache made %d channel requests", n)
	}
}

func TestChannelListUnknownGuildExits4(t *testing.T) {
	r := channelRunner(t)
	res := r.Run("channel", "list", "--guild", "nowhere")
	if res.ExitCode != 4 {
		t.Errorf("exit %d: %s", res.ExitCode, res.Stderr)
	}
}
