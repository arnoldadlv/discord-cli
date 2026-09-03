package cli_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/clitest"
)

func removeToken(t *testing.T, r *clitest.Runner) {
	t.Helper()
	if err := os.Remove(filepath.Join(r.Home.ToolConfigDir(), "token")); err != nil {
		t.Fatal(err)
	}
}

func TestAuthSetFromPipeWritesPrivateFile(t *testing.T) {
	r := clitest.NewRunner(t)
	removeToken(t, r)
	r.Stdin = strings.NewReader("secret-token-abc\n")
	res := r.Run("auth", "set")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	path := filepath.Join(r.Home.ToolConfigDir(), "token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %o, want 0600", info.Mode().Perm())
	}
	if got := strings.TrimSpace(string(r.Home.ReadFile(t, path))); got != "secret-token-abc" {
		t.Errorf("stored %q", got)
	}
	if strings.Contains(res.Stdout+res.Stderr, "secret-token-abc") {
		t.Errorf("token leaked to output: %q %q", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Token stored") {
		t.Errorf("stderr should confirm: %q", res.Stderr)
	}
}

func TestAuthSetFromTerminalPromptsWithEchoOff(t *testing.T) {
	r := clitest.NewRunner(t)
	removeToken(t, r)
	r.StdinTTY = true
	r.Password = "prompted-token-xyz"
	res := r.Run("auth", "set")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Paste your user token") {
		t.Errorf("no prompt on stderr: %q", res.Stderr)
	}
	got := strings.TrimSpace(string(r.Home.ReadFile(t, filepath.Join(r.Home.ToolConfigDir(), "token"))))
	if got != "prompted-token-xyz" {
		t.Errorf("stored %q", got)
	}
	if strings.Contains(res.Stdout+res.Stderr, "prompted-token-xyz") {
		t.Errorf("token echoed: %q %q", res.Stdout, res.Stderr)
	}
}

func TestAuthSetRejectsEmptyToken(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Stdin = strings.NewReader("\n")
	res := r.Run("auth", "set")
	if res.ExitCode != 2 {
		t.Errorf("exit %d, want 2: %s", res.ExitCode, res.Stderr)
	}
}

func TestAuthStatusFromFile(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("auth", "status")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "tester") || !strings.Contains(res.Stdout, "token file") {
		t.Errorf("stdout: %q", res.Stdout)
	}
	if strings.Contains(res.Stdout+res.Stderr, "user-token-0001") {
		t.Errorf("token printed")
	}

	var j struct {
		Source   string `json:"source"`
		Path     string `json:"path"`
		Username string `json:"username"`
		UserID   string `json:"user_id"`
		Valid    bool   `json:"valid"`
	}
	r.Run("auth", "status", "--json").JSON(t, &j)
	if j.Source != "file" || j.Username != "tester" || j.UserID != "100" || !j.Valid {
		t.Errorf("json: %+v", j)
	}
	if !strings.HasSuffix(j.Path, filepath.Join("discord-cli", "token")) {
		t.Errorf("path: %q", j.Path)
	}
}

func TestEnvironmentTokenFallbackWithNotice(t *testing.T) {
	r := clitest.NewRunner(t)
	removeToken(t, r)
	r.Env["DISCORD_TOKEN"] = "env-token-42"
	res := r.Run("auth", "status")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "DISCORD_TOKEN") {
		t.Errorf("stdout should name the source: %q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "auth set") {
		t.Errorf("stderr should suggest auth set: %q", res.Stderr)
	}
	if strings.Contains(res.Stdout+res.Stderr, "env-token-42") {
		t.Errorf("token printed")
	}
	reqs := r.Fake.RequestsTo("/users/@me")
	if len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "env-token-42" {
		t.Errorf("requests: %+v", reqs)
	}
	var j struct {
		Source string `json:"source"`
	}
	r.Run("auth", "status", "--json").JSON(t, &j)
	if j.Source != "environment" {
		t.Errorf("source %q", j.Source)
	}
}

func TestNoTokenAnywhereExits3(t *testing.T) {
	r := clitest.NewRunner(t)
	removeToken(t, r)
	res := r.Run("auth", "status")
	if res.ExitCode != 3 {
		t.Errorf("exit %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "auth set") || !strings.Contains(res.Stderr, "DISCORD_TOKEN") {
		t.Errorf("stderr: %q", res.Stderr)
	}
	if len(r.Fake.Requests()) != 0 {
		t.Errorf("should not call Discord without a token")
	}
}

func TestBotTokenIsRefused(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.JSON("/users/@me", map[string]any{"id": "7", "username": "somebot", "bot": true})
	res := r.Run("auth", "status")
	if res.ExitCode != 3 {
		t.Errorf("exit %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "bot") {
		t.Errorf("stderr: %q", res.Stderr)
	}
}

func TestRejectedTokenExits3(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.Queue("/users/@me", clitest.Response{Status: 401, Body: `{"message":"401: Unauthorized","code":0}`})
	res := r.Run("auth", "status")
	if res.ExitCode != 3 {
		t.Errorf("exit %d, want 3: %s", res.ExitCode, res.Stderr)
	}
	if n := len(r.Fake.RequestsTo("/users/@me")); n != 1 {
		t.Errorf("401 must not be retried, got %d requests", n)
	}
	var j struct {
		Valid bool `json:"valid"`
	}
	_ = j
}

func TestForbiddenIsNotRetried(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.Queue("/users/@me", clitest.Response{Status: 403, Body: `{"message":"403: Forbidden","code":50001}`})
	res := r.Run("auth", "status")
	if res.ExitCode == 0 {
		t.Errorf("expected failure")
	}
	if n := len(r.Fake.RequestsTo("/users/@me")); n != 1 {
		t.Errorf("403 must not be retried, got %d requests", n)
	}
	if len(r.Sleeps) != 0 {
		t.Errorf("no sleeps expected, got %v", r.Sleeps)
	}
}

func TestRateLimitThenSuccess(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.Queue("/users/@me",
		clitest.Response{Status: 429, Body: `{"message":"You are being rate limited.","retry_after":2.5,"global":false}`},
		clitest.Response{Status: 200, Body: map[string]any{"id": "100", "username": "tester", "bot": false}},
	)
	res := r.Run("auth", "status")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	if len(r.Sleeps) != 1 || r.Sleeps[0].Seconds() != 2.5 {
		t.Errorf("sleeps %v, want [2.5s]", r.Sleeps)
	}
	if n := len(r.Fake.RequestsTo("/users/@me")); n != 2 {
		t.Errorf("requests %d, want 2", n)
	}
}

func TestFiveRateLimitsExhaust(t *testing.T) {
	r := clitest.NewRunner(t)
	limited := clitest.Response{Status: 429, Body: `{"message":"You are being rate limited.","retry_after":1,"global":false}`}
	r.Fake.Queue("/users/@me", limited, limited, limited, limited, limited,
		clitest.Response{Status: 200, Body: map[string]any{"id": "100", "username": "tester"}})
	res := r.Run("auth", "status")
	if res.ExitCode != 6 {
		t.Errorf("exit %d, want 6: %s", res.ExitCode, res.Stderr)
	}
	if n := len(r.Fake.RequestsTo("/users/@me")); n != 5 {
		t.Errorf("requests %d, want 5", n)
	}
}

func TestRateLimitBodyWithoutRetryAfterSleepsOneSecond(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.Queue("/users/@me",
		clitest.Response{Status: 429, Body: `{"message":"You are being rate limited."}`},
		clitest.Response{Status: 200, Body: map[string]any{"id": "100", "username": "tester"}},
	)
	r.Run("auth", "status")
	if len(r.Sleeps) != 1 || r.Sleeps[0].Seconds() != 1 {
		t.Errorf("sleeps %v, want [1s]", r.Sleeps)
	}
}

func TestRemainingZeroSleepsResetAfterPlusBufferCapped(t *testing.T) {
	cases := []struct {
		reset string
		want  float64
	}{
		{"3.5", 4.5},
		{"120", 60},
	}
	for _, c := range cases {
		r := clitest.NewRunner(t)
		h := http.Header{}
		h.Set("X-RateLimit-Remaining", "0")
		h.Set("X-RateLimit-Reset-After", c.reset)
		r.Fake.Queue("/users/@me", clitest.Response{Status: 200, Header: h, Body: map[string]any{"id": "100", "username": "tester"}})
		res := r.Run("auth", "status")
		if res.ExitCode != 0 {
			t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
		}
		if len(r.Sleeps) != 1 || r.Sleeps[0].Seconds() != c.want {
			t.Errorf("reset %s: sleeps %v, want [%vs]", c.reset, r.Sleeps, c.want)
		}
	}
}

func TestRemainingNonZeroDoesNotSleep(t *testing.T) {
	r := clitest.NewRunner(t)
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "4")
	h.Set("X-RateLimit-Reset-After", "3")
	r.Fake.Queue("/users/@me", clitest.Response{Status: 200, Header: h, Body: map[string]any{"id": "100", "username": "tester"}})
	r.Run("auth", "status")
	if len(r.Sleeps) != 0 {
		t.Errorf("sleeps %v", r.Sleeps)
	}
}

func TestTimeoutIsHonored(t *testing.T) {
	r := clitest.NewRunner(t)
	r.Fake.Queue("/users/@me", clitest.Response{Status: 200, Delay: 300 * time.Millisecond, Body: map[string]any{"id": "100", "username": "tester"}})
	res := r.Run("auth", "status", "--timeout", "50ms")
	if res.ExitCode != 1 {
		t.Errorf("exit %d, want 1: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "timed out") || !strings.Contains(res.Stderr, "--timeout") {
		t.Errorf("stderr: %q", res.Stderr)
	}
}

func TestEveryRequestCarriesBrowserHeaders(t *testing.T) {
	r := clitest.NewRunner(t)
	res := r.Run("auth", "status")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Stderr)
	}
	reqs := r.Fake.RequestsTo("/users/@me")
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	h := reqs[0].Header
	want := map[string]string{
		"Authorization":      "user-token-0001",
		"User-Agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
		"X-Discord-Locale":   "en-US",
		"X-Debug-Options":    "bugReporterEnabled",
		"Sec-Ch-Ua":          `"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"macOS"`,
		"Referer":            "https://discord.com/channels/@me",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if h.Get("X-Discord-Timezone") == "" {
		t.Errorf("missing X-Discord-Timezone")
	}
	sp := h.Get("X-Super-Properties")
	raw, err := base64.StdEncoding.DecodeString(sp)
	if err != nil {
		t.Fatalf("X-Super-Properties not base64: %v", err)
	}
	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		t.Fatalf("X-Super-Properties not JSON: %v", err)
	}
	if props["client_build_number"] != float64(523497) || props["browser"] != "Chrome" || props["os"] != "Mac OS X" || props["browser_user_agent"] != want["User-Agent"] {
		t.Errorf("super properties: %v", props)
	}
	if reqs[0].Method != "GET" {
		t.Errorf("method %s", reqs[0].Method)
	}
}
