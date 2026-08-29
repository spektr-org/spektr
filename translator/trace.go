package translator

import (
	"fmt"
	"os"
)

// ============================================================================
// TRACE — evidence instrumentation (gated by SPEKTR_TRACE)
// ============================================================================
// When SPEKTR_TRACE is set, the translator prints exactly what crosses the
// trust boundary to the AI: the full data summary (metadata only) and the
// full prompt. When unset, every call is a no-op.
//
// The point being evidenced: the ONLY thing the AI receives is schema
// structure + distinct dimension LABELS + the user's question. Measure
// values and raw records are never placed in the payload.
// ============================================================================

var traceEnabled = os.Getenv("SPEKTR_TRACE") != ""

func trace(format string, args ...interface{}) {
	if !traceEnabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[TRACE translator] "+format+"\n", args...)
}