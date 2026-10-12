package engine

// Tests for logging.go — the engine's log lines carry no amounts, and the
// host can redirect or silence them.

import (
	"fmt"
	"strings"
	"testing"
)

// capture runs fn with the engine's lines collected, and restores the logger.
func capture(t *testing.T, fn func()) []string {
	t.Helper()
	var lines []string
	SetLogger(func(format string, args ...interface{}) { lines = append(lines, fmt.Sprintf(format, args...)) })
	defer func() { logf = defaultLogf }()
	fn()
	return lines
}

var defaultLogf = logf

// Progress and difference figures stay in the result, never in the log: a
// pocket's spend or a project's outstanding amount is the user's data.
func TestLogging_NoAmounts(t *testing.T) {
	lines := capture(t, func() {
		if _, err := Execute(progressSpec(), cyclingView(),
			WithReferences(map[string]Reference{"distance_goal": {Value: 2000}})); err != nil {
			t.Fatal(err)
		}
		if _, err := Execute(QuerySpec{
			Intent: "text", Aggregation: "difference", Measure: "amount",
			Minuend:    &Operand{Filters: &Filters{Dimensions: map[string][]string{"field": {"ToReceive"}}}},
			Subtrahend: &Operand{Filters: &Filters{Dimensions: map[string][]string{"field": {"Received"}}}},
		}, receivablesView()); err != nil {
			t.Fatal(err)
		}
	})
	if len(lines) == 0 {
		t.Fatal("expected the engine's lines to reach the host's logger")
	}
	// The figures involved: 900 ridden of 2,000; 300,000 invoiced, 120,000
	// received, 180,000 outstanding — in any formatting.
	for _, l := range lines {
		for _, amount := range []string{"900", "2,000", "2000", "300,000", "300000", "120,000", "120000", "180,000", "180000"} {
			if strings.Contains(l, amount) {
				t.Errorf("log line carries an amount (%s): %q", amount, l)
			}
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Progress — 45.0% attained") || !strings.Contains(joined, "Difference — ") {
		t.Errorf("the lines still say what was computed:\n%s", joined)
	}
}

func TestLogging_NilSilences(t *testing.T) {
	SetLogger(nil)
	defer func() { logf = defaultLogf }()
	if _, err := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}})); err != nil {
		t.Fatal(err)
	}
}