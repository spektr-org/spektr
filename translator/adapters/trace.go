package adapters

import (
	"fmt"
	"os"
)

// ============================================================================
// TRACE — evidence instrumentation (gated by SPEKTR_TRACE)
// ============================================================================
// Adapters are the ONLY code in Spektr that performs network I/O. When
// SPEKTR_TRACE is set, each adapter logs the single boundary-crossing HTTP
// call — proving that egress happens here and nowhere else, and showing the
// exact bytes sent and the raw response returned (a QuerySpec, not a query).
// ============================================================================

var traceEnabled = os.Getenv("SPEKTR_TRACE") != ""

func trace(format string, args ...interface{}) {
	if !traceEnabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[TRACE adapters] "+format+"\n", args...)
}