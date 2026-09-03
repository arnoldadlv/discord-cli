package cli

import (
	"fmt"
	"strings"
	"time"
)

// parseDate accepts YYYY-MM-DD (local midnight) or an RFC 3339 timestamp.
func parseDate(flag, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", value, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, UsageError("--%s must be a date like 2026-01-31 or a timestamp like 2026-01-31T09:00:00Z, not %q", flag, value)
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	if strings.HasSuffix(word, "ch") || strings.HasSuffix(word, "s") || strings.HasSuffix(word, "x") {
		return fmt.Sprintf("%d %ses", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
