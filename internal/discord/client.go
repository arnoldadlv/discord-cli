// Package discord is the client for Discord's user-account API: the same
// endpoints the official web client and DiscordChatExporter use, never the
// bot API. See ADR-0001 and docs/research for the sourced facts.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is Discord's v10 API.
const DefaultBaseURL = "https://discord.com/api/v10"

const (
	maxAttempts      = 5
	resetBuffer      = time.Second
	maxAdvisorySleep = 60 * time.Second
)

// StatusError is a non-2xx response that was not retried.
type StatusError struct {
	Status  int
	Path    string
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Discord returned %d for %s: %s", e.Status, e.Path, e.Message)
	}
	return fmt.Sprintf("Discord returned %d for %s", e.Status, e.Path)
}

// ErrRateLimitExhausted is returned after five consecutive 429 responses.
var ErrRateLimitExhausted = errors.New("rate limit exhausted after five attempts")

// ErrBotToken is returned when the token belongs to a bot account.
var ErrBotToken = errors.New("this token belongs to a bot account; discord-cli only works with a user token")

// TimeoutError is returned when one request exceeds the per-request timeout.
type TimeoutError struct {
	Path    string
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("request to %s timed out after %s", e.Path, e.Timeout)
}

// Client talks to Discord as the user's own account.
type Client struct {
	BaseURL  string
	Token    string
	Timezone string
	Timeout  time.Duration
	Sleep    func(time.Duration)
	// Notice, when set, receives one-line progress notes such as rate-limit waits.
	Notice func(string)

	http    *http.Client
	user    *User
	checked bool
}

// New builds a client. Timeout is per request; Sleep is used for every wait.
func New(baseURL, token string, timeout time.Duration, sleep func(time.Duration)) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Token:    token,
		Timezone: LocalTimezone(),
		Timeout:  timeout,
		Sleep:    sleep,
		http:     &http.Client{Timeout: timeout},
	}
}

// User is the part of the current-user object the tool needs.
type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Bot        bool   `json:"bot"`
}

// CurrentUser fetches /users/@me once and rejects bot accounts.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	if c.user != nil {
		return c.user, nil
	}
	var u User
	if err := c.get(ctx, "/users/@me", nil, &u); err != nil {
		return nil, err
	}
	c.checked = true
	if u.Bot {
		return nil, ErrBotToken
	}
	c.user = &u
	return c.user, nil
}

// Get performs one GET, checking the token kind first on the run's first call.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	if !c.checked {
		if _, err := c.CurrentUser(ctx); err != nil {
			return err
		}
	}
	return c.get(ctx, path, query, out)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header = Headers(c.Token, c.Timezone)
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var ne interface{ Timeout() bool }
			if errors.As(err, &ne) && ne.Timeout() {
				return &TimeoutError{Path: path, Timeout: c.Timeout}
			}
			return fmt.Errorf("request to %s failed: %w", path, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("reading response from %s: %w", path, readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryAfter(body, resp.Header)
			if attempt == maxAttempts {
				break
			}
			if c.Notice != nil {
				c.Notice(fmt.Sprintf("Rate limited, waiting %.1fs...", wait.Seconds()))
			}
			c.Sleep(wait)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &StatusError{Status: resp.StatusCode, Path: path, Message: apiMessage(body)}
		}

		c.honorAdvisory(resp.Header)

		if out != nil && len(strings.TrimSpace(string(body))) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decoding response from %s: %w", path, err)
			}
		}
		return nil
	}
	return ErrRateLimitExhausted
}

// retryAfter reads the wait from a 429 body, defaulting to one second.
func retryAfter(body []byte, h http.Header) time.Duration {
	var b struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &b); err == nil && b.RetryAfter > 0 {
		return time.Duration(b.RetryAfter * float64(time.Second))
	}
	if v := h.Get("Retry-After"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return time.Duration(f * float64(time.Second))
		}
	}
	return time.Second
}

// honorAdvisory sleeps out the bucket when the last request used it up:
// reset-after plus one second, capped at 60 seconds, like DiscordChatExporter.
func (c *Client) honorAdvisory(h http.Header) {
	if h.Get("X-RateLimit-Remaining") != "0" {
		return
	}
	f, err := strconv.ParseFloat(h.Get("X-RateLimit-Reset-After"), 64)
	if err != nil || f < 0 {
		return
	}
	wait := time.Duration(math.Min(f+resetBuffer.Seconds(), maxAdvisorySleep.Seconds()) * float64(time.Second))
	c.Sleep(wait)
}

func apiMessage(body []byte) string {
	var b struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &b); err == nil && b.Message != "" {
		return b.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// DisplayName is the user's display name, else the handle.
func (u User) DisplayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}
