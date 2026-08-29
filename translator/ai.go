package translator

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// AI TRANSLATOR — Natural language → QuerySpec
// ============================================================================
// AITranslator owns two things only:
//   1. Building the prompt from schema + query
//   2. Parsing the AI's text response into a QuerySpec
//
// It knows nothing about HTTP, providers, API keys, or request formats.
// All of that is the consumer's AIProvider adapter.
// ============================================================================

// AITranslator translates natural language queries into QuerySpecs.
// Delegates all provider communication to the AIProvider adapter
// supplied by the consumer.
type AITranslator struct {
	provider AIProvider
}

// NewTranslator creates an AITranslator backed by the given AIProvider.
// The consumer constructs and passes their adapter.
//
// Example using the built-in GeminiAdapter:
//
//	t := translator.NewTranslator(
//	    translator.NewGeminiAdapter("AIza...", "gemini-2.5-flash-lite"),
//	)
//
// Example using a custom adapter (e.g. TPL's relay, OpenAI, local LLM):
//
//	t := translator.NewTranslator(myCustomAdapter)
func NewTranslator(provider AIProvider) *AITranslator {
	return &AITranslator{provider: provider}
}

// Translate converts a natural language query into a QuerySpec.
// Implements the Translator interface.
func (t *AITranslator) Translate(query string, sch schema.Config) (*TranslateResult, error) {
	return t.TranslateWithSummary(query, sch, nil)
}

// TranslateWithSummary converts a query using a pre-built DataSummary.
// Prefer this over Translate when records are already parsed — the summary
// gives the AI better context for filtering and value matching.
func (t *AITranslator) TranslateWithSummary(query string, sch schema.Config, summary *DataSummary) (*TranslateResult, error) {
	// 1. Build schema-driven prompt
	prompt := BuildPrompt(sch, summary)

	// 2. Annotate ratio queries
	lower := strings.ToLower(query)
	for _, kw := range []string{"percentage of", "% of", "how much of", "portion of", "fraction of", "what part of"} {
		if strings.Contains(lower, kw) {
			prompt += "\nHINT: This is a RATIO query. Use aggregation:\"ratio\" with BOTH \"filters\" (denominator) AND \"compareFilters\" (numerator).\n"
			break
		}
	}

	prompt += "\n\nUSER QUERY: " + query + "\n\nRespond with valid JSON only:"

	log.Printf("🔄 Spektr Translator: query=\"%s\" schema=\"%s\"", truncate(query, 80), sch.Name)

	// ── EVIDENCE: this is the COMPLETE payload that crosses the trust boundary ──
	// Everything the AI ever sees is in this string. Inspect it: schema names,
	// distinct dimension labels, and the question — no measure values, no rows.
	trace("========================= BOUNDARY CROSS =========================")
	trace("Sending %d bytes to AI provider. FULL prompt (verbatim) follows:", len(prompt))
	trace("---------------- BEGIN PROMPT (all bytes to AI) ----------------")
	trace("%s", prompt)
	trace("----------------- END PROMPT (all bytes to AI) -----------------")
	trace("Note: the payload above is the ONLY data leaving the local process.")

	// 3. Delegate to the consumer's AI provider — Spektr never touches HTTP here
	response, err := t.provider.Complete(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI provider error: %w", err)
	}

	// 4. Parse response into QuerySpec
	result, err := parseResponse(response)
	if err != nil {
		log.Printf("⚠️ Spektr Translator: parse failed, using fallback: %v", err)
		return &TranslateResult{
			QuerySpec: engine.QuerySpec{
				Intent:      "table",
				Aggregation: "list",
				Visualize:   "table",
				Title:       "Query Results",
				Confidence:  0.5,
			},
			Interpretation: *parseFallbackInterpretation(response),
		}, nil
	}

	log.Printf("✅ Spektr Translator: intent=%s, visualize=%s, confidence=%.2f",
		result.QuerySpec.Intent, result.QuerySpec.Visualize, result.QuerySpec.Confidence)

	// ── EVIDENCE: what the AI returned is a declarative QuerySpec, NOT an
	// executable query (no SQL). Its fields are drawn from a closed vocabulary. ──
	trace("AI returned a QuerySpec (declarative instruction, NOT an executable query):")
	trace("  intent=%q aggregation=%q measure=%q groupBy=%v sortBy=%q limit=%d visualize=%q",
		result.QuerySpec.Intent, result.QuerySpec.Aggregation, result.QuerySpec.Measure,
		result.QuerySpec.GroupBy, result.QuerySpec.SortBy, result.QuerySpec.Limit,
		result.QuerySpec.Visualize)
	trace("  filters=%v", result.QuerySpec.Filters.Dimensions)
	trace("  reply(template, unresolved)=%q", result.QuerySpec.Reply)
	trace("No query string was produced; the struct above is executed directly by the engine.")

	return result, nil
}

// Boundary-payload limits. These bound what leaves the local process:
//   - A dimension whose distinct-value count is a high fraction of the record
//     count is a per-row IDENTIFIER / surrogate key (e.g. record_id). Its COLUMN
//     NAME is still exposed so the AI knows the column exists, but its per-row
//     VALUES are suppressed — never sent — because no analytical query filters
//     on a surrogate row key, and the values are per-row data.
//     (Repeating grouping identifiers like playbookId, where distinct << rows,
//     are NOT identifier-cardinality and keep their sample values normally.)
//   - Remaining dimensions send at most maxSampleValues representative labels,
//     so a high-cardinality column (e.g. dates) cannot leak a near-per-row set.
const (
	maxSampleValues       = 25   // cap on distinct labels sent per dimension
	identifierCardinality = 0.90 // distinct/count ratio at/above which a dim is a per-row identifier
)

// BuildDataSummaryFromRecords creates a lightweight DataSummary from records.
// Only a BOUNDED set of dimension sample values and the record count are included.
// Per-row identifier VALUES are suppressed (column name only); measure values and
// raw records are never sent.
func BuildDataSummaryFromRecords(records []engine.Record, sch schema.Config) *DataSummary {
	if len(records) == 0 {
		return &DataSummary{RecordCount: 0, Dimensions: map[string][]string{}}
	}

	seen := make(map[string]map[string]bool)
	sensitiveDim := make(map[string]bool) // PII dims: values suppressed regardless of cardinality
	for _, d := range sch.Dimensions {
		seen[d.Key] = make(map[string]bool)
		if d.Sensitive {
			sensitiveDim[d.Key] = true
		}
	}
	for _, r := range records {
		for key, val := range r.Dimensions {
			if _, ok := seen[key]; ok && val != "" {
				seen[key][val] = true
			}
		}
	}

	summary := &DataSummary{
		RecordCount: len(records),
		Dimensions:  make(map[string][]string),
	}

	var suppressed []string // identifier/PII columns: name kept, values suppressed
	var capped []string     // dimensions truncated to maxSampleValues

	for key, valSet := range seen {
		distinct := len(valSet)

		// Sensitive / PII dimension (name, email, picture_url, …): values are
		// suppressed at the SOURCE by schema discovery, regardless of cardinality.
		// This is the reliable privacy control — not the cardinality heuristic below.
		if sensitiveDim[key] {
			summary.Dimensions[key] = []string{}
			suppressed = append(suppressed, fmt.Sprintf("%s (PII/sensitive — values suppressed)", key))
			continue
		}

		// Per-row identifier / surrogate key: keep the column NAME (empty value
		// list) so the AI knows it exists, but DO NOT send its per-row values.
		if float64(distinct) >= identifierCardinality*float64(len(records)) {
			summary.Dimensions[key] = []string{} // name exposed, values suppressed
			suppressed = append(suppressed, fmt.Sprintf("%s (%d distinct — values suppressed)", key, distinct))
			continue
		}

		vals := make([]string, 0, distinct)
		for v := range valSet {
			vals = append(vals, v)
		}
		sort.Strings(vals) // deterministic sample — reproducible across runs
		// Cap high-cardinality dimensions to a representative sample.
		if len(vals) > maxSampleValues {
			vals = vals[:maxSampleValues]
			capped = append(capped, fmt.Sprintf("%s (%d→%d)", key, distinct, maxSampleValues))
		}
		summary.Dimensions[key] = vals
	}

	// ── EVIDENCE: the summary is a BOUNDED set of category labels only.
	// Per-row identifier VALUES are suppressed (name only); high-cardinality dims
	// are capped; no measure values and no per-row data cross the boundary. ──
	trace("Built DataSummary from %d records — this is ALL the data-derived context the AI gets:", len(records))
	trace("  recordCount = %d", summary.RecordCount)
	if len(suppressed) > 0 {
		trace("  IDENTIFIER columns — name exposed, per-row VALUES SUPPRESSED: %v", suppressed)
	}
	if len(capped) > 0 {
		trace("  CAPPED high-cardinality dimensions to %d sample labels: %v", maxSampleValues, capped)
	}
	for k, vals := range summary.Dimensions {
		if len(vals) == 0 {
			trace("  dimension %q -> (identifier: column name only, 0 values sent)", k)
		} else {
			trace("  dimension %q -> %d label(s) sent: %v", k, len(vals), vals)
		}
	}
	trace("  measures included in summary = NONE (no amounts, no per-row values)")
	trace("  per-row identifier VALUES included = NONE (suppressed above)")

	return summary
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}