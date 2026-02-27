#!/bin/bash
# ============================================================================
# spektr-analyze.sh — Run multiple queries against any CSV
# ============================================================================
#
# Usage:
#   ./spektr-analyze.sh sales.csv
#   ./spektr-analyze.sh jira-export.csv /c/reports/output.csv
#   ./spektr-analyze.sh data.csv results.csv gemini-2.5-flash-lite
#
# Args:
#   $1 — Input CSV file (required)
#   $2 — Output CSV file (default: ./spektr-results.csv)
#   $3 — Gemini model (default: gemini-2.5-flash-lite)
#
# Prerequisites:
#   - spektr binary built: go build -o ./bin/spektr ./cmd/spektr/
#   - GEMINI_API_KEY env var set
# ============================================================================

set -e

# ── Config ─────────────────────────────────────────────────────────────────
INPUT="${1:?Usage: ./spektr-analyze.sh <input.csv> [output.csv] [model]}"
OUTPUT="${2:-./spektr-results.csv}"
MODEL="${3:-gemini-2.5-flash-lite}"

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

if [ -z "$GEMINI_API_KEY" ]; then
    echo "Error: GEMINI_API_KEY not set"
    exit 1
fi

if [ ! -f "$INPUT" ]; then
    echo "Error: File not found: $INPUT"
    exit 1
fi

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  Spektr Analyzer                                        ║"
echo "╠══════════════════════════════════════════════════════════╣"
echo "║  Input:  $INPUT"
echo "║  Output: $OUTPUT"
echo "║  Model:  $MODEL"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# ── Step 1: Discover schema ────────────────────────────────────────────────
echo "🔍 Discovering schema..."
SCHEMA=$($SPEKTR --file "$INPUT" --discover --format pretty --model "$MODEL" 2>/dev/null)

# Extract dimension and measure names for smart query generation
DIMS=$(echo "$SCHEMA" | grep '"key"' | head -20 | sed 's/.*"key": "//;s/".*//' | tr '\n' ',' | sed 's/,$//')
echo "   Dimensions: $DIMS"

MEASURES=$(echo "$SCHEMA" | grep -A2 '"measures"' | grep '"key"' | head -10 | sed 's/.*"key": "//;s/".*//' | tr '\n' ',' | sed 's/,$//')
echo "   Measures: $MEASURES"
echo ""

# ── Step 2: Define queries ─────────────────────────────────────────────────
# These are generic queries that work across any domain.
# The AI translator adapts them to the actual schema.
QUERIES=(
    "show pie chart by assignee"
)

# ── Step 3: Run queries and build output ───────────────────────────────────
# Clear output file and write header
echo "Query,Summary" > "$OUTPUT"

PASS=0
FAIL=0

for QUERY in "${QUERIES[@]}"; do
    echo "📊 Query: $QUERY"

    # Run query, capture CSV to temp file
    TMPFILE=$(mktemp)
    if $SPEKTR --file "$INPUT" --query "$QUERY" --format csv --model "$MODEL" --out "$TMPFILE" 2>/dev/null; then
        # Check if we got real data (more than just a header row)
        LINES=$(wc -l < "$TMPFILE")
        if [ "$LINES" -gt 1 ]; then
            echo "   ✅ Got $((LINES - 1)) data rows"

            # Append a separator row, then the query results
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
done

echo ""
echo "════════════════════════════════════════════════════════════"
echo "  Done! $PASS queries succeeded, $FAIL failed"
echo "  Results: $OUTPUT"
echo "════════════════════════════════════════════════════════════"