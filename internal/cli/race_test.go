//go:build race

package cli_test

// raceEnabled is true when the race detector is on, which slows the code
// about tenfold and makes timing assertions meaningless.
const raceEnabled = true
