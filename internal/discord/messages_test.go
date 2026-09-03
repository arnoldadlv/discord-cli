package discord_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
)

// aroundServer answers the token check and records every messages request.
func aroundServer(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users/@me" {
			_, _ = w.Write([]byte(`{"id":"100","username":"tester","bot":false}`))
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(`[{"id":"5000002","content":"two","timestamp":"2026-08-01T10:02:00.000000+00:00","author":{"id":"9001","username":"ana"},"attachments":[],"embeds":[]}]`))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestMessagesRejectsAroundWithBeforeOrAfter(t *testing.T) {
	var hits atomic.Int64
	s := aroundServer(t, &hits)
	c := discord.New(s.URL, "user-token-0001", "UTC", 5*time.Second, nil)
	ctx := context.Background()

	for _, tc := range []struct{ name, before, after string }{
		{"before", "5000005", ""},
		{"after", "", "5000001"},
	} {
		_, err := c.Messages(ctx, "2001", 5, tc.before, tc.after, "5000003")
		if err == nil {
			t.Fatalf("%s: around with %s was accepted", tc.name, tc.name)
		}
		if !strings.Contains(err.Error(), "around") {
			t.Errorf("%s: error does not name around: %v", tc.name, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("a rejected call still reached Discord %d times", hits.Load())
	}
}

func TestMessagesSendsAroundAlone(t *testing.T) {
	var hits atomic.Int64
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users/@me" {
			_, _ = w.Write([]byte(`{"id":"100","username":"tester","bot":false}`))
			return
		}
		hits.Add(1)
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{"id":"5000002","content":"two","timestamp":"2026-08-01T10:02:00.000000+00:00","author":{"id":"9001","username":"ana"},"attachments":[],"embeds":[]}]`))
	}))
	defer s.Close()
	c := discord.New(s.URL, "user-token-0001", "UTC", 5*time.Second, nil)
	ms, err := c.Messages(context.Background(), "2001", 5, "", "", "5000002")
	if err != nil {
		t.Fatalf("around alone: %v", err)
	}
	if len(ms) != 1 || ms[0].ID != "5000002" {
		t.Errorf("messages: %+v", ms)
	}
	if !strings.Contains(got, "around=5000002") || strings.Contains(got, "before=") || strings.Contains(got, "after=") {
		t.Errorf("query: %q", got)
	}
	if hits.Load() != 1 {
		t.Errorf("requests: %d", hits.Load())
	}
}
