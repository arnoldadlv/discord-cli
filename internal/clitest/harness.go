// Package clitest is the test harness for driving the discord command through
// its command boundary: a fake Discord HTTP server, a temporary home with XDG
// directories, and a runner that captures stdout, stderr, and the exit code.
package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/cli"
)

// Result is what a user or agent can observe after one command run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// JSON decodes stdout as JSON into v and fails the test if that is impossible.
func (r Result) JSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(r.Stdout), v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.Stdout)
	}
}

// Request is one request the fake Discord server received.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
}

// Response is one canned response the fake server will serve.
type Response struct {
	Status int
	Body   any // marshalled as JSON unless it is a string or []byte
	Header http.Header
	Delay  time.Duration
}

// Handler computes a response from a request; used for paged endpoints.
type Handler func(r *http.Request) Response

// FakeDiscord is an httptest server that serves canned responses and records
// every request it receives.
type FakeDiscord struct {
	Server *httptest.Server

	mu          sync.Mutex
	routes      map[string]Handler
	queues      map[string][]Response
	requests    []Request
	inFlight    int
	maxInFlight int
}

// NewFakeDiscord starts a fake server; it is closed when the test ends.
func NewFakeDiscord(t testing.TB) *FakeDiscord {
	t.Helper()
	f := &FakeDiscord{routes: map[string]Handler{}, queues: map[string][]Response{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

// URL is the base URL to give the tool in place of Discord's API.
func (f *FakeDiscord) URL() string { return f.Server.URL }

// Handle serves every request for path through h.
func (f *FakeDiscord) Handle(path string, h Handler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[path] = h
}

// JSON serves a fixed 200 JSON body for path.
func (f *FakeDiscord) JSON(path string, body any) {
	f.Handle(path, func(*http.Request) Response { return Response{Status: 200, Body: body} })
}

// Queue serves the given responses for path in order; the last one repeats.
func (f *FakeDiscord) Queue(path string, responses ...Response) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queues[path] = append(f.queues[path], responses...)
}

// Requests returns a copy of every request received so far.
func (f *FakeDiscord) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Request, len(f.requests))
	copy(out, f.requests)
	return out
}

// RequestsTo returns the requests whose path equals path.
func (f *FakeDiscord) RequestsTo(path string) []Request {
	var out []Request
	for _, r := range f.Requests() {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// MaxInFlight is the largest number of requests that were being served at once.
func (f *FakeDiscord) MaxInFlight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

// Reset forgets recorded requests but keeps routes.
func (f *FakeDiscord) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
	f.maxInFlight = 0
}

func (f *FakeDiscord) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone()})
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	var resp Response
	var found bool
	if q, ok := f.queues[r.URL.Path]; ok && len(q) > 0 {
		resp = q[0]
		if len(q) > 1 {
			f.queues[r.URL.Path] = q[1:]
		}
		found = true
	} else if h, ok := f.routes[r.URL.Path]; ok {
		f.mu.Unlock()
		resp = h(r)
		f.mu.Lock()
		found = true
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	if !found {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"404: Not Found","code":0}`))
		return
	}
	if resp.Delay > 0 {
		time.Sleep(resp.Delay)
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	status := resp.Status
	if status == 0 {
		status = 200
	}
	switch b := resp.Body.(type) {
	case nil:
		w.WriteHeader(status)
	case string:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(b))
	case []byte:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(b)
	}
}

// Home is a temporary home directory with XDG directories beneath it.
type Home struct {
	Dir       string
	ConfigDir string // XDG_CONFIG_HOME
	DataDir   string // XDG_DATA_HOME
	CacheDir  string // XDG_CACHE_HOME
}

// NewHome creates a temporary home for one test.
func NewHome(t testing.TB) *Home {
	t.Helper()
	dir := t.TempDir()
	h := &Home{
		Dir:       dir,
		ConfigDir: filepath.Join(dir, ".config"),
		DataDir:   filepath.Join(dir, ".local", "share"),
		CacheDir:  filepath.Join(dir, ".cache"),
	}
	for _, d := range []string{h.ConfigDir, h.DataDir, h.CacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

// ToolConfigDir is where the tool keeps its config (token, config.json).
func (h *Home) ToolConfigDir() string { return filepath.Join(h.ConfigDir, "discord-cli") }

// ToolDataDir is where the tool keeps its data (exports).
func (h *Home) ToolDataDir() string { return filepath.Join(h.DataDir, "discord-cli") }

// ExportsDir is where new exports are written.
func (h *Home) ExportsDir() string { return filepath.Join(h.ToolDataDir(), "exports") }

// ToolCacheDir is where the tool keeps its lookup cache and search index.
func (h *Home) ToolCacheDir() string { return filepath.Join(h.CacheDir, "discord-cli") }

// LegacyExportsDir is the Node CLI's export directory under this home.
func (h *Home) LegacyExportsDir() string { return filepath.Join(h.Dir, ".discord-cli", "exports") }

// DCEExportsDir is the DiscordChatExporter folder under this home.
func (h *Home) DCEExportsDir() string {
	return filepath.Join(h.Dir, "DiscordChatExporter.Cli.osx-arm64", "exports")
}

// WriteFile writes a file under the home, creating directories as needed.
func (h *Home) WriteFile(t testing.TB, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

// ReadFile reads a file under the home.
func (h *Home) ReadFile(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Runner runs the tool against a fake server and a home.
type Runner struct {
	T      testing.TB
	Fake   *FakeDiscord
	Home   *Home
	Env    map[string]string
	Stdin  io.Reader
	Sleeps []time.Duration
	Now    func() time.Time

	// Terminal flags let a test pretend a stream is a TTY.
	StdinTTY, StdoutTTY, StderrTTY bool
}

// NewRunner builds a runner with a fresh fake server and home, and a token
// already stored so commands can talk to the fake server.
func NewRunner(t testing.TB) *Runner {
	t.Helper()
	r := &Runner{T: t, Fake: NewFakeDiscord(t), Home: NewHome(t), Env: map[string]string{}}
	r.Fake.JSON("/users/@me", map[string]any{"id": "100", "username": "tester", "bot": false})
	r.SetToken("user-token-0001")
	return r
}

// SetToken writes a token file as `auth set` would.
func (r *Runner) SetToken(token string) {
	r.Home.WriteFile(r.T, filepath.Join(r.Home.ToolConfigDir(), "token"), []byte(token+"\n"), 0o600)
}

// Run executes one command line and returns what it produced.
func (r *Runner) Run(args ...string) Result {
	r.T.Helper()
	var stdout, stderr bytes.Buffer
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return r.Home.Dir
		case "XDG_CONFIG_HOME":
			return r.Home.ConfigDir
		case "XDG_DATA_HOME":
			return r.Home.DataDir
		case "XDG_CACHE_HOME":
			return r.Home.CacheDir
		}
		return r.Env[k]
	}
	stdin := r.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	env := cli.Env{
		Args:             args,
		Stdin:            stdin,
		Stdout:           &stdout,
		Stderr:           &stderr,
		Getenv:           getenv,
		StdinIsTerminal:  r.StdinTTY,
		StdoutIsTerminal: r.StdoutTTY,
		StderrIsTerminal: r.StderrTTY,
		Sleep:            func(d time.Duration) { r.Sleeps = append(r.Sleeps, d) },
		APIBaseURL:       r.Fake.URL(),
		Now:              now,
		Version:          "test",
	}
	code := cli.Run(context.Background(), env)
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}
