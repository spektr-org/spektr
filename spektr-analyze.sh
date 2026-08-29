#!/bin/bash
# ============================================================================
# spektr-analyze.sh — Run queries against any CSV, with optional --trace
# ============================================================================
#
# Usage:
#   ./spektr-analyze.sh test.csv
#   ./spektr-analyze.sh test.csv --trace
#   ./spektr-analyze.sh test.csv results.csv gemini-2.5-flash-lite --trace
#
# --trace sets SPEKTR_TRACE=1, which makes the engine/translator/adapter
# packages print a step-by-step evidence log to stderr proving:
#   • exactly what crosses the trust boundary to the AI (schema + labels only),
#   • that the AI returns a declarative QuerySpec, not an executable query,
#   • that all computation happens locally with NO query and NO network,
#   • that records are read zero-copy (indices, never copied or sent).
#
# When --trace is absent, behaviour is identical to the untraced library.
#
# Args (order-flexible for --trace; positional otherwise):
#   $1 — Input CSV file (required)
#   $2 — Output CSV file (default: ./spektr-results.csv)
#   $3 — Gemini model     (default: gemini-2.5-flash-lite)
#   --trace anywhere      — enable step-by-step evidence trace
#
# Prerequisites:
#   - spektr binary built: go build -o ./bin/spektr ./cmd/spektr/
#   - AI_API_KEY env var set
# ============================================================================

set -e

# ── Parse args: pull out --trace, keep the rest positional ─────────────────
TRACE=0
POSITIONAL=()
for arg in "$@"; do
    case "$arg" in
        --trace) TRACE=1 ;;
        *) POSITIONAL+=("$arg") ;;
    esac
done
set -- "${POSITIONAL[@]}"

INPUT="${1:?Usage: ./spektr-analyze.sh <input.csv> [output.csv] [model] [--trace]}"
OUTPUT="${2:-./spektr-results.csv}"
MODEL="${3:-gemini-2.5-flash-lite}"

# ── Enable evidence trace if requested ─────────────────────────────────────
if [ "$TRACE" -eq 1 ]; then
    export SPEKTR_TRACE=1
    TRACE_LOG="./spektr-trace.log"
    : > "$TRACE_LOG"   # truncate
    echo "🧾 Trace mode ON — step-by-step evidence to stderr and $TRACE_LOG"
else
    unset SPEKTR_TRACE
fi

# Find spektr binary — check common locations
SPEKTR=""
for candidate in ./bin/spektr ./spektr ../bin/spektr; do
    if [ -x "$candidate" ]; then
        SPEKTR="$candidate"
        break
    fi
done

if [ -z "$SPEKTR" ]; then
    echo "Error: spektr binary not found. Run: go build -o ./bin/spektr ./cmd/spektr/"
    exit 1
fi

if [ -z "$AI_API_KEY" ]; then
    echo "Error: AI_API_KEY not set"
    exit 1
fi

if [ ! -f "$INPUT" ]; then
    echo "Error: File not found: $INPUT"
    exit 1
fi

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  Spektr Analyzer                                         ║"
echo "╠══════════════════════════════════════════════════════════╣"
echo "║  Input:  $INPUT"
echo "║  Output: $OUTPUT"
echo "║  Model:  $MODEL"
echo "║  Trace:  $([ "$TRACE" -eq 1 ] && echo ON || echo off)"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# Helper: run the binary, tee-ing stderr (trace) to the log when tracing.
run_spektr() {
    if [ "$TRACE" -eq 1 ]; then
        # stderr carries the [TRACE ...] lines — show them AND append to the log
        "$@" 2> >(tee -a "$TRACE_LOG" >&2)
    else
        "$@" 2>/dev/null
    fi
}

# ── Step 1: Discover schema ────────────────────────────────────────────────
echo "🔍 Discovering schema..."
SCHEMA=$(run_spektr $SPEKTR --file "$INPUT" --discover --format pretty --model "$MODEL")

DIMS=$(echo "$SCHEMA" | grep '"key"' | head -20 | sed 's/.*"key": "//;s/".*//' | tr '\n' ',' | sed 's/,$//')
echo "   Dimensions: $DIMS"
MEASURES=$(echo "$SCHEMA" | grep -A2 '"measures"' | grep '"key"' | head -10 | sed 's/.*"key": "//;s/".*//' | tr '\n' ',' | sed 's/,$//')
echo "   Measures: $MEASURES"
echo ""

# ── Step 2: Define queries ─────────────────────────────────────────────────
# Multiple generic queries over the domain-neutral test.csv. Each exercises a
# different path (filter+group+sum, count, avg, max, ratio) so the trace shows
# the full range of the local execution pipeline.
QUERIES=(
    #Queries that can be run on exhibit 1 - exhibit_1_Non_finance_domain
	#"total energy for faulty devices by region"
    #"how many records per device type"
    #"average duration by vendor"
    #"which region has the highest energy usage"
    #"total units by status"
	
	#Queries that can be run on exhibit 2 - exhibit_2_personal_finance_domain
	# --- TABLE outputs (grouped breakdowns) ---
    "show me a table of total spend by category"
    "break down spending by payment method"
    # --- TREND outputs (time series / line) ---
    "show the monthly spending trend"
    "how did my travel spending change month over month"
    # --- TEXT outputs (single-value natural-language answers) ---
    "how much did I spend on groceries in total"
    "what was my highest spending category"
)

# ── Step 3: Run queries and build output ───────────────────────────────────
echo "Query,Summary" > "$OUTPUT"
PASS=0
FAIL=0

for QUERY in "${QUERIES[@]}"; do
    echo "📊 Query: $QUERY"
    if [ "$TRACE" -eq 1 ]; then
        echo "───────────────────────── TRACE ─────────────────────────" >&2
        echo "QUERY: $QUERY" >&2
    fi

    TMPFILE=$(mktemp)
    if run_spektr $SPEKTR --file "$INPUT" --query "$QUERY" --format csv --model "$MODEL" --out "$TMPFILE"; then
        LINES=$(wc -l < "$TMPFILE")
        if [ "$LINES" -gt 1 ]; then
            echo "   ✅ Got $((LINES - 1)) data rows"
            echo "" >> "$OUTPUT"
            echo "--- $QUERY ---" >> "$OUTPUT"
            cat "$TMPFILE" >> "$OUTPUT"
            PASS=$((PASS + 1))
        else
            echo "   ⚠️  No data rows returned"
            FAIL=$((FAIL + 1))
        fi
    else
        echo "   ❌ Query failed"
        FAIL=$((FAIL + 1))
    fi
    rm -f "$TMPFILE"
    echo ""
done

echo "════════════════════════════════════════════════════════════"
echo "  Done! $PASS queries succeeded, $FAIL failed"
echo "  Results: $OUTPUT"
[ "$TRACE" -eq 1 ] && echo "  Full evidence trace: $TRACE_LOG"
echo "════════════════════════════════════════════════════════════"