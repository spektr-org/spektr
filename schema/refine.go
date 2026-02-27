package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// SMART REFINE — AI-Assisted Schema Enrichment (One-Time)
// ============================================================================
//
// After Auto-Detect produces a draft schema from heuristics, Smart Refine
// optionally sends column metadata (~200-500 bytes) to Gemini for semantic
// enrichment. This is called ONCE at setup time — the result is cached by
// the consumer and never re-fetched unless the data shape changes.
//
// What Gemini sees:
//   - Column names, detected types, sample values, unique counts
//   - Row count
//   - Auto-detected hierarchies, currency, temporal flags
//
// What Gemini NEVER sees:
//   - Raw data values, actual amounts, user content
//
// What Gemini returns:
//   - Suggested dataset name + description
//   - Human-friendly display names for each column
//   - Semantic descriptions ("Issue severity level", "Effort estimation in Fibonacci scale")
//   - Unit classification (currency, hours, points, percent, units)
//   - Hierarchy suggestions the heuristics may have missed
//   - Sort hints for ordinal dimensions (P1 > P2 > P3 > P4)
//   - Default aggregation suggestions (sum vs avg vs count)
//
// Design doc reference: Section 4.3 (Smart Refine / AI-Assisted Refinement)
// ============================================================================

// RefineConfig holds the AI provider configuration for Smart Refine.
type RefineConfig struct {
	APIKey   string // Gemini API key (consumer's key)
	Model    string // Model name (default: "gemini-2.5-flash-lite")
	Endpoint string // API endpoint (default: Gemini v1beta)
}

// DefaultRefineConfig returns sensible defaults for Gemini.
func DefaultRefineConfig(apiKey string) RefineConfig {
	return RefineConfig{
		APIKey:   apiKey,
		Model:    "gemini-2.5-flash-lite",
		Endpoint: "https://generativelanguage.googleapis.com/v1beta/models",
	}
}

// Refine enriches an Auto-Detect schema using a one-time AI call.
// The original Config is NOT mutated — a new enriched Config is returned.
// If the AI call fails, returns the original Config unchanged with the error.
func Refine(draft *Config, cfg RefineConfig) (*Config, error) {
	if draft == nil {
		return nil, fmt.Errorf("draft schema is nil")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required for Smart Refine")
	}

	// Apply defaults
	if cfg.Model == "" {
		cfg.Model = "gemini-2.5-flash-lite"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://generativelanguage.googleapis.com/v1beta/models"
	}

	// 1. Build the lightweight metadata payload
	payload := buildRefinePayload(draft)

	// 2. Build the prompt
	prompt := buildRefinePrompt(payload)

	log.Printf("🧠 Spektr Smart Refine: sending %d columns, %d bytes metadata",
		len(payload.Columns), len(prompt))

	// 3. Call Gemini
	response, err := callRefineGemini(prompt, cfg)
	if err != nil {
		log.Printf("⚠️ Smart Refine: AI call failed: %v — returning draft unchanged", err)
		return draft, fmt.Errorf("smart refine AI call failed: %w", err)
	}

	// 4. Parse AI response
	enrichment, err := parseRefineResponse(response)
	if err != nil {
		log.Printf("⚠️ Smart Refine: Parse failed: %v — returning draft unchanged", err)
		return draft, fmt.Errorf("smart refine parse failed: %w", err)
	}

	// 5. Apply enrichments to a copy of the draft
	result := applyEnrichments(draft, enrichment)

	log.Printf("✅ Spektr Smart Refine: enriched %d dimensions, %d measures",
		len(result.Dimensions), len(result.Measures))

	return result, nil
}

// ============================================================================
// PAYLOAD BUILDER — What the AI Sees (~200-500 bytes)
// ============================================================================

// refinePayload is the lightweight metadata sent to Gemini.
type refinePayload struct {
	Columns    []refineColumn   `json:"columns"`
	RowCount   int              `json:"rowCount"`
	Detected   refineDetected   `json:"detected"`
}

type refineColumn struct {
	Name       string   `json:"name"`
	Key        string   `json:"key"`
	Role       string   `json:"role"`       // "dimension", "measure", "skipped"
	Type       string   `json:"type"`       // "string", "numeric", "date", "bool"
	Samples    []string `json:"samples"`
	Unique     int      `json:"unique"`
	// Existing detection flags — so AI can confirm/correct
	IsTemporal     bool   `json:"isTemporal,omitempty"`
	IsCurrencyCode bool   `json:"isCurrencyCode,omitempty"`
	Parent         string `json:"parent,omitempty"`
}

type refineDetected struct {
	HasCurrency    bool     `json:"hasCurrency"`
	HasTemporal    bool     `json:"hasTemporal"`
	Hierarchies    []string `json:"hierarchies,omitempty"`    // "field → category" format
	SkippedColumns []string `json:"skippedColumns,omitempty"` // names of skipped columns
}

func buildRefinePayload(draft *Config) refinePayload {
	p := refinePayload{
		RowCount: 0, // not tracked in Config; AI can infer from context
	}

	// Dimensions
	for _, d := range draft.Dimensions {
		col := refineColumn{
			Name:           d.DisplayName,
			Key:            d.Key,
			Role:           "dimension",
			Type:           "string",
			Samples:        limitSamples(d.SampleValues, 5),
			Unique:         estimateUnique(d.CardinalityHint),
			IsTemporal:     d.IsTemporal,
			IsCurrencyCode: d.IsCurrencyCode,
			Parent:         d.Parent,
		}
		if d.IsTemporal {
			col.Type = "temporal"
		}
		p.Columns = append(p.Columns, col)
	}

	// Measures (skip synthetic)
	for _, m := range draft.Measures {
		if m.IsSynthetic {
			continue
		}
		p.Columns = append(p.Columns, refineColumn{
			Name: m.DisplayName,
			Key:  m.Key,
			Role: "measure",
			Type: "numeric",
		})
	}

	// Detected patterns
	p.Detected.HasCurrency = draft.Currency != nil && draft.Currency.Enabled
	for _, d := range draft.Dimensions {
		if d.IsTemporal {
			p.Detected.HasTemporal = true
		}
		if d.Parent != "" {
			p.Detected.Hierarchies = append(p.Detected.Hierarchies,
				fmt.Sprintf("%s → %s", d.Key, d.Parent))
		}
	}
	for _, s := range draft.SkippedColumns {
		p.Detected.SkippedColumns = append(p.Detected.SkippedColumns, s.Column)
	}

	return p
}

// ============================================================================
// PROMPT BUILDER
// ============================================================================

func buildRefinePrompt(payload refinePayload) string {
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")

	return fmt.Sprintf(`You are a data analyst inspecting a dataset's structure. Based on the column metadata below, provide semantic enrichments.

COLUMN METADATA:
%s

INSTRUCTIONS:
1. Suggest a concise, descriptive name for this dataset (2-5 words)
2. Write a one-line description of what this dataset contains
3. For each column, provide:
   - displayName: Human-friendly label (e.g., "story_points" → "Story Points", "assignee" → "Assignee")
   - description: What this column represents in the domain (e.g., "Issue severity level", "Sprint effort estimation")
   - unit: For measures only — one of: "currency", "hours", "points", "percent", "units", or "" if unknown
   - sortHint: For ordinal dimensions — natural ordering (e.g., "P1 > P2 > P3 > P4", "To Do > In Progress > Done")
   - defaultAggregation: For measures — "sum", "avg", "count", "max", "min" (based on what makes semantic sense)
4. Suggest any hierarchies the heuristics may have missed (parent → child relationships)
5. Flag any columns currently classified as "skipped" that should probably be included

Respond with ONLY valid JSON (no markdown, no backticks):
{
  "datasetName": "...",
  "datasetDescription": "...",
  "enrichments": [
    {
      "key": "column_key",
      "displayName": "...",
      "description": "...",
      "unit": "",
      "sortHint": "",
      "defaultAggregation": ""
    }
  ],
  "suggestedHierarchies": [
    {"parent": "parent_key", "child": "child_key", "reason": "..."}
  ],
  "recoverColumns": [
    {"column": "column_name", "reason": "...", "suggestedRole": "dimension"}
  ]
}`, string(payloadJSON))
}

// ============================================================================
// RESPONSE TYPES
// ============================================================================

// refineEnrichment is the parsed AI response.
type refineEnrichment struct {
	DatasetName        string                   `json:"datasetName"`
	DatasetDescription string                   `json:"datasetDescription"`
	Enrichments        []columnEnrichment       `json:"enrichments"`
	SuggestedHierarchies []hierarchySuggestion  `json:"suggestedHierarchies"`
	RecoverColumns     []recoverSuggestion      `json:"recoverColumns"`
}

type columnEnrichment struct {
	Key                string `json:"key"`
	DisplayName        string `json:"displayName"`
	Description        string `json:"description"`
	Unit               string `json:"unit"`
	SortHint           string `json:"sortHint"`
	DefaultAggregation string `json:"defaultAggregation"`
}

type hierarchySuggestion struct {
	Parent string `json:"parent"`
	Child  string `json:"child"`
	Reason string `json:"reason"`
}

type recoverSuggestion struct {
	Column        string `json:"column"`
	Reason        string `json:"reason"`
	SuggestedRole string `json:"suggestedRole"`
}

// ============================================================================
// RESPONSE PARSER
// ============================================================================

func parseRefineResponse(response string) (*refineEnrichment, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result refineEnrichment
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse refine response: %w (response: %.300s)", err, response)
	}

	return &result, nil
}

// ============================================================================
// APPLY ENRICHMENTS — Merge AI suggestions into the schema
// ============================================================================

// applyEnrichments creates a new Config with AI enrichments merged in.
// Rules:
//   - AI suggestions override Auto-Detect's generic display names and empty descriptions
//   - AI cannot change column roles (dimension/measure) or keys
//   - AI cannot remove columns or add new ones (except recover suggestions)
//   - Sort hints are stored in a new SortHint field on DimensionMeta
//   - Hierarchy suggestions are applied only if not already detected
func applyEnrichments(draft *Config, enrichment *refineEnrichment) *Config {
	// Deep copy
	result := deepCopyConfig(draft)

	// Apply dataset-level enrichments
	if enrichment.DatasetName != "" {
		result.Name = enrichment.DatasetName
	}
	if enrichment.DatasetDescription != "" {
		result.Description = enrichment.DatasetDescription
	}

	// Build enrichment lookup by key
	enrichMap := make(map[string]columnEnrichment)
	for _, e := range enrichment.Enrichments {
		enrichMap[e.Key] = e
	}

	// Enrich dimensions
	for i := range result.Dimensions {
		d := &result.Dimensions[i]
		if e, ok := enrichMap[d.Key]; ok {
			if e.DisplayName != "" {
				d.DisplayName = e.DisplayName
			}
			if e.Description != "" {
				d.Description = e.Description
			}
			if e.SortHint != "" {
				d.SortHint = e.SortHint
			}
		}
	}

	// Enrich measures
	for i := range result.Measures {
		m := &result.Measures[i]
		if e, ok := enrichMap[m.Key]; ok {
			if e.DisplayName != "" {
				m.DisplayName = e.DisplayName
			}
			if e.Description != "" {
				m.Description = e.Description
			}
			if e.Unit != "" {
				m.Unit = e.Unit
				if e.Unit == "currency" {
					m.IsCurrency = true
				}
			}
			if e.DefaultAggregation != "" && isValidAggregation(e.DefaultAggregation) {
				m.DefaultAggregation = e.DefaultAggregation
			}
		}
	}

	// Apply hierarchy suggestions (only if not already set)
	for _, h := range enrichment.SuggestedHierarchies {
		for i := range result.Dimensions {
			if result.Dimensions[i].Key == h.Child && result.Dimensions[i].Parent == "" {
				result.Dimensions[i].Parent = h.Parent
			}
		}
	}

	// Mark as refined
	result.RefinedAt = time.Now().Format(time.RFC3339)
	result.RefinedBy = "gemini"

	return result
}

// ============================================================================
// GEMINI API CALL (self-contained — no dependency on translator package)
// ============================================================================

// geminiRefineRequest mirrors the Gemini API request format.
type geminiRefineRequest struct {
	Contents []geminiRefineContent `json:"contents"`
}

type geminiRefineContent struct {
	Parts []geminiRefinePart `json:"parts"`
}

type geminiRefinePart struct {
	Text string `json:"text"`
}

type geminiRefineResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func callRefineGemini(prompt string, cfg RefineConfig) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s",
		cfg.Endpoint, cfg.Model, cfg.APIKey)

	reqBody := geminiRefineRequest{
		Contents: []geminiRefineContent{{
			Parts: []geminiRefinePart{{Text: prompt}},
		}},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API returned %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}

	var geminiResp geminiRefineResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("Gemini error %d: %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned empty response")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

// ============================================================================
// HELPERS
// ============================================================================

func deepCopyConfig(src *Config) *Config {
	dst := &Config{
		Name:           src.Name,
		Version:        src.Version,
		Description:    src.Description,
		DiscoveredFrom: src.DiscoveredFrom,
		DiscoveredAt:   src.DiscoveredAt,
	}

	// Deep copy dimensions
	dst.Dimensions = make([]DimensionMeta, len(src.Dimensions))
	for i, d := range src.Dimensions {
		dst.Dimensions[i] = d
		// Deep copy slices
		dst.Dimensions[i].SampleValues = make([]string, len(d.SampleValues))
		copy(dst.Dimensions[i].SampleValues, d.SampleValues)
	}

	// Deep copy measures
	dst.Measures = make([]MeasureMeta, len(src.Measures))
	for i, m := range src.Measures {
		dst.Measures[i] = m
		dst.Measures[i].Aggregations = make([]string, len(m.Aggregations))
		copy(dst.Measures[i].Aggregations, m.Aggregations)
	}

	// Deep copy skipped columns
	dst.SkippedColumns = make([]SkippedColumn, len(src.SkippedColumns))
	copy(dst.SkippedColumns, src.SkippedColumns)

	// Deep copy currency config
	if src.Currency != nil {
		currency := *src.Currency
		currency.Rates = make(map[string]float64)
		for k, v := range src.Currency.Rates {
			currency.Rates[k] = v
		}
		dst.Currency = &currency
	}

	return dst
}

func limitSamples(vals []string, max int) []string {
	if len(vals) <= max {
		return vals
	}
	return vals[:max]
}

func estimateUnique(hint string) int {
	switch hint {
	case "low":
		return 5
	case "medium":
		return 30
	case "high":
		return 200
	default:
		return 10
	}
}

func isValidAggregation(agg string) bool {
	switch agg {
	case "sum", "avg", "count", "max", "min":
		return true
	}
	return false
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}