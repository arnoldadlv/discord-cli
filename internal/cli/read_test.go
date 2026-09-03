package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

var messageHeader = regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2} \d{2}:\d{2}  `)

// countMessages counts rendered messages by their timestamp header line.
func countMessages(out string) int { return len(messageHeader.FindAllString(out, -1)) }

func readRunner(t *testing.T, n int) (*clitest.Runner, *clitest.MessageStore) {
	t.Helper()
	r := channelRunner(t)
	s := clitest.ServeMessages(r.Fake, "2001", clitest.Messages("2001", n))
	return r, s
}

func TestChannelReadDefault25OldestFirst(t *testing.T) {
	r, _ := readRunner(t, 40)
	res := r.Run("channel", "read", "general")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.Stdout, "message 15 ") || !strings.Contains(res.Stdout, "message 16 ") || !strings.Contains(res.Stdout, "message 40 ") {
		t.Errorf("wrong window:\n%s", res.Stdout)
	}
	if strings.Index(res.Stdout, "message 16 ") > strings.Index(res.Stdout, "message 40 ") {
		t.Errorf("not oldest first:\n%s", res.Stdout)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("limit") != "25" || reqs[0].Query.Get("before") != "" || reqs[0].Query.Get("after") != "" {
		t.Errorf("requests: %+v", reqs)
	}
}

func TestChannelReadLimitFiveAndThreePages(t *testing.T) {
	r, _ := readRunner(t, 300)
	res := r.Run("channel", "read", "general", "--limit", "5")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if countMessages(res.Stdout) != 5 || !strings.Contains(res.Stdout, "message 296 ") {
		t.Errorf("limit 5:\n%s", res.Stdout)
	}
	reqs := r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 1 || reqs[0].Query.Get("limit") != "5" {
		t.Errorf("limit 5 requests: %+v", reqs)
	}

	r.Fake.Reset()
	var j struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	res = r.Run("channel", "read", "general", "--limit", "250", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	res.JSON(t, &j)
	if len(j.Messages) != 250 || j.Messages[0].ID != clitest.MessageID(51) || j.Messages[249].ID != clitest.MessageID(300) {
		t.Errorf("250: got %d, first %s last %s", len(j.Messages), j.Messages[0].ID, j.Messages[len(j.Messages)-1].ID)
	}
	reqs = r.Fake.RequestsTo("/channels/2001/messages")
	if len(reqs) != 3 {
		t.Fatalf("want 3 pages, got %d", len(reqs))
	}
	if reqs[0].Query.Get("before") != "" || reqs[1].Query.Get("before") != clitest.MessageID(201) || reqs[2].Query.Get("before") != clitest.MessageID(101) {
		t.Errorf("before sequence: %v %v %v", reqs[0].Query, reqs[1].Query, reqs[2].Query)
	}
	for _, q := range reqs {
		if q.Query.Get("after") != "" {
			t.Errorf("after sent on a read")
		}
	}
	if reqs[0].Query.Get("limit") != "100" || reqs[2].Query.Get("limit") != "50" {
		t.Errorf("limits: %v %v", reqs[0].Query, reqs[2].Query)
	}
}

func TestChannelReadRendersAttachmentsEmbedsReactions(t *testing.T) {
	r, _ := readRunner(t, 5)
	res := r.Run("channel", "read", "📰news")
	if res.ExitCode == 0 {
		t.Fatalf("news has no messages served, expected a failure or empty; got %q", res.Stdout)
	}
	res = r.Run("channel", "read", "2001")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	out := res.Stdout
	for _, want := range []string{
		"Ana", "Kyle B", "newsbot",
		"report.pdf", "https://cdn.example.test/report.pdf",
		"Weekly digest", "Three things happened this week.", "https://news.example.test/digest",
		"👍 3", "🎉 1",
		"2026-08-01",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("embed html not stripped:\n%s", out)
	}
}

// compactMessageJSON mirrors the shape channel and DM reads emit as --json,
// for tests that decode it.
type compactMessageJSON struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Edited    bool   `json:"edited"`
	Author    struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"author"`
	Content     string                      `json:"content"`
	ReplyTo     string                      `json:"reply_to"`
	Mentions    []struct{ ID, Name string } `json:"mentions"`
	Attachments []struct {
		Filename string `json:"filename"`
		URL      string `json:"url"`
		Size     int64  `json:"size"`
	} `json:"attachments"`
	Embeds []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	} `json:"embeds"`
	Reactions []struct {
		Emoji string `json:"emoji"`
		Count int    `json:"count"`
	} `json:"reactions"`
}

func TestChannelReadJSONShapeSnapshot(t *testing.T) {
	r, _ := readRunner(t, 5)
	res := r.Run("channel", "read", "general", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	golden := filepath.Join("testdata", "channel_read.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(res.Stdout), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden missing (run with UPDATE_GOLDEN=1): %v", err)
	}
	if string(want) != res.Stdout {
		t.Errorf("JSON shape changed; diff against %s:\n%s", golden, res.Stdout)
	}
	// The golden file must not regress into Discord's raw message object:
	// none of the fields this shape drops should reappear.
	for _, dropped := range []string{"avatar_decoration_data", "display_name_styles", "referenced_message", "proxy_url", "burst_colors", "count_details", "channel_id", "mention_everyone"} {
		if strings.Contains(res.Stdout, dropped) {
			t.Errorf("compact shape leaked raw field %q", dropped)
		}
	}
	var j struct {
		Guild   struct{ ID, Name string }
		Channel struct {
			ID, Name string
			Type     int
		}
		Messages []compactMessageJSON
	}
	res.JSON(t, &j)
	if j.Guild.Name != "Cooey COE" || j.Channel.Name != "🔮general" || j.Channel.Type != 0 || len(j.Messages) != 5 {
		t.Errorf("%+v", j)
	}
	if j.Messages[0].Author.ID != "9002" || j.Messages[0].Author.Name != "Kyle B" {
		t.Errorf("author not projected to id+name: %+v", j.Messages[0].Author)
	}
	if len(j.Messages[0].Attachments) != 1 || j.Messages[0].Attachments[0].Filename != "report.pdf" || j.Messages[0].Attachments[0].URL != "https://cdn.example.test/report.pdf" || j.Messages[0].Attachments[0].Size != 1234 {
		t.Errorf("attachment: %+v", j.Messages[0].Attachments)
	}
	if len(j.Messages[1].Embeds) != 1 || j.Messages[1].Embeds[0].Title != "Weekly digest" || j.Messages[1].Embeds[0].Description != "Three things happened this week." || j.Messages[1].Embeds[0].URL != "https://news.example.test/digest" {
		t.Errorf("embed: %+v", j.Messages[1].Embeds)
	}
	if strings.Contains(j.Messages[1].Embeds[0].Description, "<b>") {
		t.Errorf("embed description html not stripped: %q", j.Messages[1].Embeds[0].Description)
	}
	if len(j.Messages[2].Reactions) != 2 || j.Messages[2].Reactions[0].Emoji != "👍" || j.Messages[2].Reactions[0].Count != 3 {
		t.Errorf("reactions: %+v", j.Messages[2].Reactions)
	}
	for i, m := range j.Messages {
		if m.Edited {
			t.Errorf("message %d: none of these fixtures are edited", i)
		}
		if len(m.Mentions) != 0 || m.ReplyTo != "" {
			t.Errorf("message %d: none of these fixtures mention or reply", i)
		}
	}
	if len(j.Messages[3].Attachments) != 0 || len(j.Messages[3].Embeds) != 0 || len(j.Messages[3].Reactions) != 0 {
		t.Errorf("plain message should omit empty arrays entirely: %+v", j.Messages[3])
	}
}

// TestChannelReadJSONUnder10KBFor20Messages pins the size win this shape
// exists for: the raw API objects measured 59 KB for 20 messages.
func TestChannelReadJSONUnder10KBFor20Messages(t *testing.T) {
	r, _ := readRunner(t, 20)
	res := r.Run("channel", "read", "general", "--limit", "20", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if n := len(res.Stdout); n >= 10*1024 {
		t.Errorf("20 messages serialised to %d bytes, want under 10 KB", n)
	}
	var j struct {
		Messages []compactMessageJSON `json:"messages"`
	}
	res.JSON(t, &j)
	if len(j.Messages) != 20 {
		t.Fatalf("got %d messages, want 20", len(j.Messages))
	}
}

// TestChannelReadJSONMentionsReplyAndEdited exercises the fields the
// fixture pool never sets: a reply, mentions, and an edited message. These
// are not shown by the human renderer, but the compact shape reads them for
// an agent that wants to follow a thread or know a message changed.
func TestChannelReadJSONMentionsReplyAndEdited(t *testing.T) {
	r := channelRunner(t)
	edited := "2026-08-01T10:09:00.000000+00:00"
	msgs := []map[string]any{
		{
			"id": clitest.MessageID(1), "channel_id": "2001", "type": 0,
			"author":           map[string]any{"id": "9001", "username": "ana", "global_name": "Ana", "discriminator": "0"},
			"content":          "welcome!",
			"timestamp":        "2026-08-01T10:01:00.000000+00:00",
			"edited_timestamp": nil,
			"attachments":      []any{}, "embeds": []any{}, "mentions": []any{},
		},
		{
			"id": clitest.MessageID(2), "channel_id": "2001", "type": 0,
			"author":            map[string]any{"id": "9002", "username": "kyle", "global_name": "Kyle B", "discriminator": "0"},
			"content":           "thanks <@9001>, will do",
			"timestamp":         edited,
			"edited_timestamp":  edited,
			"message_reference": map[string]any{"type": 0, "channel_id": "2001", "message_id": clitest.MessageID(1)},
			"mentions":          []map[string]any{{"id": "9001", "username": "ana", "global_name": "Ana", "discriminator": "0"}},
			"attachments":       []any{}, "embeds": []any{},
		},
	}
	clitest.ServeMessages(r.Fake, "2001", msgs)
	res := r.Run("channel", "read", "general", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	var j struct {
		Messages []compactMessageJSON `json:"messages"`
	}
	res.JSON(t, &j)
	if len(j.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(j.Messages))
	}
	if j.Messages[0].Edited || j.Messages[0].ReplyTo != "" || len(j.Messages[0].Mentions) != 0 {
		t.Errorf("first message should be plain: %+v", j.Messages[0])
	}
	reply := j.Messages[1]
	if !reply.Edited {
		t.Errorf("edited_timestamp set but edited is false: %+v", reply)
	}
	if reply.ReplyTo != clitest.MessageID(1) {
		t.Errorf("reply_to = %q, want %q", reply.ReplyTo, clitest.MessageID(1))
	}
	if len(reply.Mentions) != 1 || reply.Mentions[0].ID != "9001" || reply.Mentions[0].Name != "Ana" {
		t.Errorf("mentions: %+v", reply.Mentions)
	}
	if strings.Contains(res.Stdout, "referenced_message") {
		t.Errorf("reply_to must be a plain id, not an inlined referenced_message:\n%s", res.Stdout)
	}
}

// TestChannelReadHumanAndJSONShowSameFields walks both renderers over the
// same fixtures: every field the human layout prints must also be findable
// in the JSON, so an agent using --json never loses information a person
// reading the terminal would see.
func TestChannelReadHumanAndJSONShowSameFields(t *testing.T) {
	r, _ := readRunner(t, 5)
	human := r.Run("channel", "read", "general")
	if human.ExitCode != 0 {
		t.Fatalf("exit %d: %s", human.ExitCode, human.Stderr)
	}
	var j struct {
		Messages []compactMessageJSON `json:"messages"`
	}
	r.Run("channel", "read", "general", "--json").JSON(t, &j)

	// What the human layout shows for these 5 fixture messages: author,
	// content, attachment name and url, embed title/description/url, and
	// each reaction's emoji and count (see TestChannelReadRendersAttachmentsEmbedsReactions).
	authorNames := map[string]bool{}
	for _, m := range j.Messages {
		authorNames[m.Author.Name] = true
		if !strings.Contains(human.Stdout, m.Author.Name) {
			t.Errorf("human output missing author %q", m.Author.Name)
		}
		if m.Content != "" && !strings.Contains(human.Stdout, m.Content) {
			t.Errorf("human output missing content %q", m.Content)
		}
		for _, a := range m.Attachments {
			if !strings.Contains(human.Stdout, a.Filename) || !strings.Contains(human.Stdout, a.URL) {
				t.Errorf("human output missing attachment %+v", a)
			}
		}
		for _, e := range m.Embeds {
			if !strings.Contains(human.Stdout, e.Title) || !strings.Contains(human.Stdout, e.Description) || !strings.Contains(human.Stdout, e.URL) {
				t.Errorf("human output missing embed %+v", e)
			}
		}
		for _, react := range m.Reactions {
			if !strings.Contains(human.Stdout, react.Emoji) {
				t.Errorf("human output missing reaction %+v", react)
			}
		}
	}
	for _, want := range []string{"Ana", "Kyle B", "newsbot"} {
		if !authorNames[want] {
			t.Errorf("JSON never carried author %q that the human layout shows", want)
		}
	}
}

func TestChannelReadEmptyChannel(t *testing.T) {
	r, _ := readRunner(t, 0)
	res := r.Run("channel", "read", "general")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "No messages") {
		t.Errorf("%d %q %q", res.ExitCode, res.Stdout, res.Stderr)
	}
	var j struct {
		Messages []any `json:"messages"`
	}
	r.Run("channel", "read", "general", "--json").JSON(t, &j)
	if j.Messages == nil || len(j.Messages) != 0 {
		t.Errorf("messages should be an empty array")
	}
}

func TestChannelReadResolutionAndSuggestions(t *testing.T) {
	r, _ := readRunner(t, 3)
	for _, input := range []string{"2001", "🔮general", "🔮GENERAL", "general", "GENERAL"} {
		res := r.Run("channel", "read", input)
		if res.ExitCode != 0 {
			t.Errorf("%q: exit %d: %s", input, res.ExitCode, res.Stderr)
		}
	}
	res := r.Run("channel", "read", "gen")
	if res.ExitCode != 4 {
		t.Fatalf("exit %d, want 4: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, `channel "gen" not found`) || !strings.Contains(res.Stderr, "🔮general") || !strings.Contains(res.Stderr, "cmmc-general") {
		t.Errorf("stderr: %q", res.Stderr)
	}
	res = r.Run("channel", "read")
	if res.ExitCode != 2 {
		t.Errorf("missing positional: exit %d", res.ExitCode)
	}
	res = r.Run("channel", "read", "a", "b")
	if res.ExitCode != 2 {
		t.Errorf("two positionals: exit %d", res.ExitCode)
	}
}

func TestChannelReadThreadWithThreadsFlag(t *testing.T) {
	r, _ := readRunner(t, 3)
	clitest.ServeMessages(r.Fake, "3001", clitest.Messages("3001", 2))
	res := r.Run("channel", "read", "welcome thread")
	if res.ExitCode != 4 {
		t.Errorf("thread without --threads should not resolve: %d", res.ExitCode)
	}
	res = r.Run("channel", "read", "welcome thread", "--threads")
	if res.ExitCode != 0 || countMessages(res.Stdout) != 2 || !strings.Contains(res.Stdout, "message 1 ") {
		t.Errorf("exit %d: %s %s", res.ExitCode, res.Stdout, res.Stderr)
	}
}

func TestChannelReadFlagsInAnyOrder(t *testing.T) {
	r, _ := readRunner(t, 3)
	for _, args := range [][]string{
		{"channel", "read", "general", "--limit", "2"},
		{"channel", "read", "--limit", "2", "general"},
		{"--limit", "2", "channel", "read", "general"},
		{"channel", "--limit=2", "read", "general"},
	} {
		res := r.Run(args...)
		if res.ExitCode != 0 || countMessages(res.Stdout) != 2 {
			t.Errorf("%v: exit %d out %q err %q", args, res.ExitCode, res.Stdout, res.Stderr)
		}
	}
}
