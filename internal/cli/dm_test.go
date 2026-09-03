package cli_test

import (
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func dmRunner(t *testing.T) *clitest.Runner {
	t.Helper()
	r := clitest.NewRunner(t)
	r.Fake.JSON("/users/@me/channels", clitest.DMs())
	return r
}

func TestDMListHumanAndJSON(t *testing.T) {
	r := dmRunner(t)
	res := r.Run("dm", "list")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	for _, want := range []string{"kyle", "Study Group", "maria", "ana, maria", "6001", "6002", "group", "Kyle B"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("missing %q in\n%s", want, res.Stdout)
		}
	}
	var j []struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		Name         string `json:"name"`
		Participants []struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		} `json:"participants"`
	}
	r.Run("dm", "list", "--json").JSON(t, &j)
	if len(j) != 4 || j[0].Type != "dm" || j[0].Name != "kyle" || j[1].Type != "group" || j[1].Name != "Study Group" || len(j[1].Participants) != 2 || j[3].Name != "ana, maria" {
		t.Errorf("%+v", j)
	}
	if j[0].Participants[0].DisplayName != "Kyle B" {
		t.Errorf("%+v", j[0])
	}
	r.Fake.Reset()
	r.Run("dm", "list")
	if n := len(r.Fake.Requests()); n != 0 {
		t.Errorf("dm list not cached: %d requests", n)
	}
	r.Run("dm", "list", "--no-cache")
	if n := len(r.Fake.RequestsTo("/users/@me/channels")); n != 1 {
		t.Errorf("--no-cache: %d requests", n)
	}
}

func TestDMReadByUsername(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 30))
	res := r.Run("dm", "read", "kyle", "--limit", "3")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if countMessages(res.Stdout) != 3 || !strings.Contains(res.Stdout, "message 30 ") {
		t.Errorf("out: %q", res.Stdout)
	}
	var j struct {
		Channel struct {
			ID, Name string
			Type     int
		} `json:"channel"`
		Messages []struct{ ID string } `json:"messages"`
	}
	r.Run("dm", "read", "Kyle B", "--json").JSON(t, &j)
	if j.Channel.ID != "6001" || j.Channel.Name != "kyle" || j.Channel.Type != 1 || len(j.Messages) != 25 {
		t.Errorf("%+v", j)
	}
}

func TestDMResolution(t *testing.T) {
	r := dmRunner(t)
	for _, id := range []string{"6001", "6002", "6003", "6004"} {
		clitest.ServeMessages(r.Fake, id, clitest.Messages(id, 1))
	}
	cases := map[string]string{
		"kyle":        "6001", // the DM wins over the group kyle is in
		"6002":        "6002",
		"study group": "6002",
		"Study-Group": "6002",
		"maria":       "6003",
		"Maria":       "6003",
		"ana, maria":  "6004",
	}
	for input, want := range cases {
		var j struct {
			Channel struct{ ID string } `json:"channel"`
		}
		res := r.Run("dm", "read", input, "--json")
		if res.ExitCode != 0 {
			t.Errorf("%q: exit %d: %s", input, res.ExitCode, res.Stderr)
			continue
		}
		res.JSON(t, &j)
		if j.Channel.ID != want {
			t.Errorf("%q -> %s, want %s", input, j.Channel.ID, want)
		}
	}
	// ana is only in groups: two of them, so the participant match is ambiguous.
	res := r.Run("dm", "read", "ana")
	if res.ExitCode != 4 || !strings.Contains(res.Stderr, "Study Group") || !strings.Contains(res.Stderr, "ana, maria") {
		t.Errorf("ambiguous: exit %d %q", res.ExitCode, res.Stderr)
	}
	res = r.Run("dm", "read", "nobody")
	if res.ExitCode != 4 || !strings.Contains(res.Stderr, `DM "nobody" not found`) {
		t.Errorf("unknown: exit %d %q", res.ExitCode, res.Stderr)
	}
	res = r.Run("dm", "read", "mar")
	if res.ExitCode != 4 || !strings.Contains(res.Stderr, "maria") {
		t.Errorf("suggestion: exit %d %q", res.ExitCode, res.Stderr)
	}
}

func TestDMSearchFetchesHistoryAndFilters(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 150))
	res := r.Run("dm", "search", "kyle", "--query", "policy", "--limit", "5")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	reqs := r.Fake.RequestsTo("/channels/6001/messages")
	if len(reqs) != 2 || reqs[1].Query.Get("before") != clitest.MessageID(51) {
		t.Errorf("history requests: %+v", reqs)
	}
	// "policy" is topic n%3==1; embed-only messages (n%5==2) have no content.
	if countMessages(res.Stdout) != 5 || !strings.Contains(res.Stdout, "message 148 ") {
		t.Errorf("out: %q", res.Stdout)
	}
	if strings.Index(res.Stdout, "message 148 ") > strings.Index(res.Stdout, "message 145 ") {
		t.Errorf("not newest first")
	}
	if !strings.Contains(res.Stdout, "40 matches") {
		t.Errorf("match count missing: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "dm export") || !strings.Contains(res.Stderr, "export search") || !strings.Contains(res.Stderr, "150") {
		t.Errorf("notice missing: %q", res.Stderr)
	}
	var j struct {
		Channel      struct{ ID string } `json:"channel"`
		TotalMatches int                 `json:"total_matches"`
		Messages     []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	r.Run("dm", "search", "kyle", "policy", "--json").JSON(t, &j)
	if j.TotalMatches != 40 || len(j.Messages) != 25 || j.Messages[0].ID != clitest.MessageID(148) {
		t.Errorf("%+v", j)
	}
	for _, m := range j.Messages {
		if !strings.Contains(m.Content, "policy") {
			t.Errorf("unfiltered: %q", m.Content)
		}
	}
}

func TestDMSearchAnyTermAndDates(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 30))
	var j struct {
		TotalMatches int `json:"total_matches"`
	}
	r.Run("dm", "search", "kyle", "--query", "policy scoping", "--json").JSON(t, &j)
	if j.TotalMatches != 16 { // 20 policy/scoping messages minus 4 embed-only ones
		t.Errorf("any term: %d", j.TotalMatches)
	}
	// Messages 1..30 run 10:01 to 10:30 UTC on 2026-08-01.
	res := r.Run("dm", "search", "kyle", "--query", "message", "--after", "2026-08-01T10:10:00Z", "--before", "2026-08-01T10:20:00Z", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if j.TotalMatches != 9 {
		t.Errorf("date window: %d", j.TotalMatches)
	}
	res = r.Run("dm", "search", "kyle", "--query", "message", "--after", "2026-08-02", "--json")
	res.JSON(t, &j)
	if j.TotalMatches != 0 {
		t.Errorf("after date: %d", j.TotalMatches)
	}
	res = r.Run("dm", "search", "kyle", "--query", "x", "--after", "yesterday")
	if res.ExitCode != 2 {
		t.Errorf("bad date: exit %d", res.ExitCode)
	}
	if strings.Contains(res.Stderr, "dm export") {
		t.Errorf("no notice for a single page history: %q", res.Stderr)
	}
}

func TestDMSearchSinglePageNoNotice(t *testing.T) {
	r := dmRunner(t)
	clitest.ServeMessages(r.Fake, "6001", clitest.Messages("6001", 30))
	res := r.Run("dm", "search", "kyle", "--query", "zzz")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "No matches") || res.Stderr != "" {
		t.Errorf("exit %d out %q err %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	res = r.Run("dm", "search", "kyle")
	if res.ExitCode != 2 {
		t.Errorf("missing query: exit %d", res.ExitCode)
	}
}
