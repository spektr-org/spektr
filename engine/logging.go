package engine

import "log"

// logf is where the engine's progress lines go: the standard logger unless
// the host says otherwise. The lines name what was computed (intent, filters,
// labels, percentages) and never the amounts themselves: a host that keeps
// amounts out of its logs, as a privacy-first finance app must, can leave
// logging on.
var logf = log.Printf

// SetLogger sends the engine's log lines to f instead of the standard
// logger. nil silences them. Set it once at start-up, before any Execute:
// it is not synchronised with running queries.
func SetLogger(f func(format string, args ...interface{})) {
	if f == nil {
		f = func(string, ...interface{}) {}
	}
	logf = f
}