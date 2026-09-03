package discord_test

import (
	"context"
	"testing"
	"time"

	"github.com/arnoldadlv/discord-cli/internal/discord"
)

// A rate-limit wait must end as soon as the run is interrupted.
func TestContextSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	discord.ContextSleep(ctx, 10*time.Second)
	if time.Since(start) > 2*time.Second {
		t.Errorf("sleep ignored the cancelled context")
	}
	if utf := discord.Truncate("héllo wörld", 4); utf != "héll..." {
		t.Errorf("Truncate = %q", utf)
	}
}
