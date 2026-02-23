package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spektr-org/spektr/engine"
)

// ============================================================================
// RESPONSE PARSER — Extracts QuerySpec from AI response
// ============================================================================
// TPL origin: nlm.go → parseQuerySpecResponse + parseInterpretResponse
// Key change: QueryFilters{Categories,Locations,...} → Filters{Dimensions: map}
// ============================================================================

// parseResponse extracts TranslateResult from the AI's JSON response.
func parseResponse(response string) (*TranslateResult, error) {
	// Clean up response — remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result TranslateResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse translator response: %w (response: %.200s)", err, response)
	}

	// Apply defaults for missing fields
	if result.QuerySpec.Intent == "" {
		result.QuerySpec.Intent = "text"
	}
	if result.QuerySpec.Aggregation == "" {
		result.QuerySpec.Aggregation = "sum"
	}
	if result.QuerySpec.Visualize == "" {
		result.QuerySpec.Visualize = result.QuerySpec.Intent
	}

	// Sync confidence
	if result.QuerySpec.Confidence == 0 && result.Interpretation.Confidence > 0 {
		result.QuerySpec.Confidence = result.Interpretation.Confidence
	}

	// Normalize via engine rules
	result.QuerySpec = engine.NormalizeQuerySpec(result.QuerySpec)

	return &result, nil
}

// parseFallbackInterpretation tries to extract just the Interpretation.
// Used when the full response parse fails.
func parseFallbackInterpretation(response string) *engine.Interpretation {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Try wrapped format: {"interpretation": {...}}
	var wrapper struct {
		Interpretation engine.Interpretation `json:"interpretation"`
	}
	if err := json.Unmarshal([]byte(response), &wrapper); err == nil && wrapper.Interpretation.Summary != "" {
		return &wrapper.Interpretation
	}

	// Try direct format
	var direct engine.Interpretation
	if err := json.Unmarshal([]byte(response), &direct); err == nil && direct.Summary != "" {
		return &direct
	}

	// Return generic low-confidence fallback
	return &engine.Interpretation{
		VisualType: "table",
		Summary:    "I'll try to show results for your query",
		Details: []engine.InterpretDetail{
			{Label: "Display", Value: "Data table"},
		},
		Confidence: 0.5,
	}
}
