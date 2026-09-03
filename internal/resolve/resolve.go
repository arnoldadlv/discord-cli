// Package resolve turns a name or id typed by a person into one guild,
// channel, or DM, with the three-step rule from the spec: numeric input is an
// id; then an exact case-insensitive match on the raw name; then a match on
// normalised names; then failure with suggestions.
package resolve

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Candidate is one thing a name can resolve to.
type Candidate struct {
	ID   string
	Name string
	// AlsoNames are other primary names of the candidate itself, such as
	// the display name of the person behind a DM. They rank with Name.
	AlsoNames []string
	// Aliases are weaker names that also identify the candidate, such as the
	// participants of a group DM. They are tried after the primary names.
	Aliases []string
}

// NotFoundError is returned when nothing matched.
type NotFoundError struct {
	Kind        string // "guild", "channel", "DM"
	Input       string
	Suggestions []string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Kind, e.Input)
}

// AmbiguousError is returned when the normalised step matched several.
type AmbiguousError struct {
	Kind       string
	Input      string
	Candidates []Candidate
}

func (e *AmbiguousError) Error() string {
	names := make([]string, len(e.Candidates))
	for i, c := range e.Candidates {
		names[i] = fmt.Sprintf("%s (%s)", c.Name, c.ID)
	}
	return fmt.Sprintf("%s %q matches more than one: %s", e.Kind, e.Input, strings.Join(names, ", "))
}

var (
	numeric    = regexp.MustCompile(`^[0-9]+$`)
	whitespace = regexp.MustCompile(`\s+`)
	nonName    = regexp.MustCompile(`[^a-z0-9-]`)
)

// IsID reports whether the input is a numeric id.
func IsID(s string) bool { return numeric.MatchString(s) }

// Normalize lowercases, turns whitespace runs into hyphens, and drops every
// character outside [a-z0-9-], which strips emoji prefixes. It is the rule
// the Node CLI used for file and directory names, kept for compatibility.
func Normalize(name string) string {
	s := strings.ToLower(name)
	s = whitespace.ReplaceAllString(s, "-")
	s = nonName.ReplaceAllString(s, "")
	return s
}

// Key is the normalised form used for matching: Normalize with the hyphens
// that a stripped emoji prefix or suffix leaves behind trimmed away.
func Key(name string) string {
	return strings.Trim(Normalize(name), "-")
}

// Match resolves input against the candidates. After the id step, primary
// names are tried before aliases at each of the exact and normalised
// steps, so a DM with a person beats a group that person is in.
func Match(kind, input string, candidates []Candidate) (Candidate, error) {
	input = strings.TrimSpace(input)
	if IsID(input) {
		for _, c := range candidates {
			if c.ID == input {
				return c, nil
			}
		}
		// An id we do not know is still an id; let the caller ask Discord.
		return Candidate{ID: input, Name: input}, nil
	}

	lower := strings.ToLower(input)
	norm := Key(input)
	exact := func(n string) bool { return strings.ToLower(n) == lower }
	normalised := func(n string) bool { return norm != "" && Key(n) == norm }
	steps := []func(c Candidate) bool{
		func(c Candidate) bool { return anyMatch(c.primary(), exact) },
		func(c Candidate) bool { return anyMatch(c.Aliases, exact) },
		func(c Candidate) bool { return anyMatch(c.primary(), normalised) },
		func(c Candidate) bool { return anyMatch(c.Aliases, normalised) },
	}
	for _, step := range steps {
		var hits []Candidate
		for _, c := range candidates {
			if step(c) {
				hits = append(hits, c)
			}
		}
		if len(hits) == 1 {
			return hits[0], nil
		}
		if len(hits) > 1 {
			return Candidate{}, &AmbiguousError{Kind: kind, Input: input, Candidates: hits}
		}
	}
	return Candidate{}, &NotFoundError{Kind: kind, Input: input, Suggestions: Suggest(input, candidates, 5)}
}

func anyMatch(names []string, f func(string) bool) bool {
	for _, n := range names {
		if f(n) {
			return true
		}
	}
	return false
}

func (c Candidate) primary() []string {
	return append([]string{c.Name}, c.AlsoNames...)
}

func (c Candidate) names() []string {
	return append(c.primary(), c.Aliases...)
}

// Suggest returns up to max candidate names that contain the input as a
// substring, case-insensitively, on the raw or normalised name.
func Suggest(input string, candidates []Candidate, max int) []string {
	lower := strings.ToLower(strings.TrimSpace(input))
	norm := Key(input)
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		for _, n := range c.names() {
			hit := strings.Contains(strings.ToLower(n), lower)
			if !hit && norm != "" {
				hit = strings.Contains(Key(n), norm)
			}
			if hit && !seen[c.Name] {
				seen[c.Name] = true
				out = append(out, c.Name)
				break
			}
		}
	}
	sort.Strings(out)
	if len(out) > max {
		out = out[:max]
	}
	return out
}
