// Package term holds the small output helpers the commands share: colour
// that is only on when a stream is a terminal, tables, and JSON.
package term

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Style colours text when enabled and returns it unchanged otherwise.
type Style struct {
	Enabled bool
}

func (s Style) wrap(code, text string) string {
	if !s.Enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Bold makes text bold.
func (s Style) Bold(text string) string { return s.wrap("1", text) }

// Dim makes text dim.
func (s Style) Dim(text string) string { return s.wrap("2", text) }

// Green colours text green.
func (s Style) Green(text string) string { return s.wrap("32", text) }

// Yellow colours text yellow.
func (s Style) Yellow(text string) string { return s.wrap("33", text) }

// Red colours text red.
func (s Style) Red(text string) string { return s.wrap("31", text) }

// Cyan colours text cyan.
func (s Style) Cyan(text string) string { return s.wrap("36", text) }

// ColorEnabled decides whether a stream should carry colour: only when it is a
// terminal, NO_COLOR is unset, TERM is not dumb, and --no-color was not given.
func ColorEnabled(isTerminal bool, getenv func(string) string, noColorFlag bool) bool {
	if noColorFlag || !isTerminal {
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	return true
}

// WriteJSON writes v as one indented JSON document followed by a newline.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Column describes one table column.
type Column struct {
	Header string
	Right  bool // right-align (numbers)
}

// Table writes rows as aligned columns separated by two spaces. The header
// row is bolded when the style is enabled.
func Table(w io.Writer, style Style, cols []Column, rows [][]string) {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c.Header)
	}
	for _, r := range rows {
		for i := range cols {
			if i < len(r) {
				if n := utf8.RuneCountInString(r[i]); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}
	line := func(cells []string, bold bool) string {
		parts := make([]string, len(cols))
		for i := range cols {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			pad := widths[i] - utf8.RuneCountInString(cell)
			if i == len(cols)-1 && !cols[i].Right {
				parts[i] = cell
				continue
			}
			if cols[i].Right {
				parts[i] = strings.Repeat(" ", pad) + cell
			} else {
				parts[i] = cell + strings.Repeat(" ", pad)
			}
		}
		s := strings.TrimRight(strings.Join(parts, "  "), " ")
		if bold {
			return style.Bold(s)
		}
		return s
	}
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}
	fmt.Fprintln(w, line(headers, true))
	for _, r := range rows {
		fmt.Fprintln(w, line(r, false))
	}
}

// TSV writes rows as tab-separated values: no padding, since it is for a
// pipeline rather than a person. noHeader suppresses the header row.
func TSV(w io.Writer, cols []Column, rows [][]string, noHeader bool) {
	if !noHeader {
		headers := make([]string, len(cols))
		for i, c := range cols {
			headers[i] = c.Header
		}
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
}
