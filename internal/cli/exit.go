package cli

import (
	"errors"
	"fmt"
	"strings"
)

// Exit codes. Agents branch on these, so they never change.
const (
	ExitOK          = 0
	ExitUnexpected  = 1
	ExitUsage       = 2
	ExitAuth        = 3
	ExitNotFound    = 4
	ExitNoExports   = 5
	ExitRateLimited = 6
)

// ExitError carries an exit code, a human message, and an optional hint
// naming what to run next.
type ExitError struct {
	Code int
	Msg  string
	Hint string
}

func (e *ExitError) Error() string {
	if e.Hint == "" {
		return e.Msg
	}
	return e.Msg + "\n" + e.Hint
}

// Errorf builds an ExitError with a formatted message.
func Errorf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// WithHint attaches a hint to the error.
func (e *ExitError) WithHint(format string, args ...any) *ExitError {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

// UsageError is an ExitError with the usage code.
func UsageError(format string, args ...any) *ExitError {
	return Errorf(ExitUsage, format, args...)
}

// exitCode maps any error to the code the process should exit with.
func exitCode(err error, ranCommand bool) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	if !ranCommand {
		return ExitUsage
	}
	return ExitUnexpected
}

// humanError formats an error for stderr: prefixed, one message per line, no
// stack trace.
func humanError(err error) string {
	msg := strings.TrimRight(err.Error(), "\n")
	lines := strings.Split(msg, "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = "discord: " + l
		} else {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
