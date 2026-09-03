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

// queryTerms splits a query into lowercase terms; a message matches when
// any term appears in its content.
func queryTerms(query string) []string {
	var terms []string
	for _, t := range strings.Fields(strings.ToLower(query)) {
		terms = append(terms, t)
	}
	return terms
}

// matchesTerms reports whether content contains any of the terms.
func matchesTerms(content string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	lower := strings.ToLower(content)
	for _, t := range terms {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// inRange reports whether t is within [after, before]; zero bounds are open.
func inRange(t, after, before time.Time) bool {
	if !after.IsZero() && t.Before(after) {
		return false
	}
	if !before.IsZero() && t.After(before) {
		return false
	}
	return true
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
