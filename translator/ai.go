package translator

import (
	"fmt"
	"log"
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

	return result, nil
}

// BuildDataSummaryFromRecords creates a lightweight DataSummary from records.
// Only dimension sample values and record count are included.
// Measure values and raw data are never sent to the AI.
func BuildDataSummaryFromRecords(records []engine.Record, sch schema.Config) *DataSummary {
	if len(records) == 0 {
		return &DataSummary{RecordCount: 0, Dimensions: map[string][]string{}}
	}

	seen := make(map[string]map[string]bool)
	for _, d := range sch.Dimensions {
		seen[d.Key] = make(map[string]bool)
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
	for key, valSet := range seen {
		vals := make([]string, 0, len(valSet))
		for v := range valSet {
			vals = append(vals, v)
		}
		summary.Dimensions[key] = vals
	}
	return summary
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}