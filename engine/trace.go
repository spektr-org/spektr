package engine

import (
	"fmt"
	"os"
)

// ============================================================================
// TRACE — evidence instrumentation (gated by SPEKTR_TRACE)
// ============================================================================
// When the SPEKTR_TRACE environment variable is set (e.g. "1"), the engine
// prints a step-by-step record of execution to stderr. When unset, every
// trace call is a no-op and behaviour is identical to the untraced library.
//
// Purpose: allow anyone (including a reviewer running the public repo) to
// verify, from a live run, that:
//   • the engine performs ALL computation locally,
//   • NO executable query (SQL or otherwise) is ever generated,
//   • records are read via zero-copy views, never copied or sent anywhere,
//   • no AI/network call occurs during execution.
//
// This file adds observability only. It changes no computation.
// ============================================================================

var traceEnabled = os.Getenv("SPEKTR_TRACE") != ""

// trace prints an evidence line to stderr when SPEKTR_TRACE is set.
func trace(format string, args ...interface{}) {
	if !traceEnabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[TRACE engine] "+format+"\n", args...)
}