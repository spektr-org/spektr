package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================================
// SMART REFINE TESTS
// ============================================================================
// Tests cover:
//   1. Payload builder — correct metadata extraction from draft Config
//   2. Response parser — valid JSON, malformed JSON, fallback
//   3. Enrichment applicator — display names, descriptions, units, hierarchies
//   4. Deep copy isolation — enriched Config doesn't mutate the draft
//   5. Edge cases — empty schema, nil Config, missing API key
// ============================================================================

// --- Test Fixtures ---

func jiraDraftSchema() *Config {
	return &Config{
		Name:           "Auto-discovered Dataset",
		Version:        "1.0",
		DiscoveredFrom: "CSV",
		Dimensions: []DimensionMeta{
			{
				Key:             "status",
				DisplayName:     "Status",
				SampleValues:    []string{"To Do", "In Progress", "Done", "Review"},
				Groupable:       true,
				Filterable:      true,
				CardinalityHint: "low",
			},
			{
				Key:             "priority",
				DisplayName:     "Priority",
				SampleValues:    []string{"P1 - Critical", "P2 - High", "P3 - Medium", "P4 - Low"},
				Groupable:       true,
				Filterable:      true,
				CardinalityHint: "low",
			},
			{
				Key:             "issue_type",
				DisplayName:     "Issue Type",
				SampleValues:    []string{"Bug", "Story", "Task", "Epic"},
				Groupable:       true,
				Filterable:      true,
				CardinalityHint: "low",
			},
			{
				Key:             "sprint",
				DisplayName:     "Sprint",
				SampleValues:    []string{"Sprint 1", "Sprint 2", "Sprint 3"},
				Groupable:       true,
				Filterable:      true,
				IsTemporal:      true,
				TemporalFormat:  "",
				CardinalityHint: "low",
			},
		},
		Measures: []MeasureMeta{
			{
				Key:                "story_points",
				DisplayName:        "Story Points",
				Aggregations:       []string{"sum", "avg", "min", "max", "count"},
				DefaultAggregation: "sum",
			},
			{
				Key:                "record_count",
				DisplayName:        "Record Count",
				IsSynthetic:        true,
				Aggregations:       []string{"count"},
				DefaultAggregation: "count",
			},
		},
		SkippedColumns: []SkippedColumn{
			{Column: "Summary", Reason: "Unique per row — likely an identifier", Recoverable: true},
		},
	}
}

func mockGeminiResponse() string {
	return `{
  "datasetName": "Jira Project Tracker",
  "datasetDescription": "Issue tracking data from a Jira project board",
  "enrichments": [
    {
      "key": "status",
      "displayName": "Status",
      "description": "Issue workflow state in the Kanban board",
      "sortHint": "To Do > In Progress > Review > Done"
    },
    {
      "key": "priority",
      "displayName": "Priority",
      "description": "Issue severity level",
      "sortHint": "P1 - Critical > P2 - High > P3 - Medium > P4 - Low"
    },
    {
      "key": "issue_type",
      "displayName": "Issue Type",
      "description": "Classification of work item (Bug, Story, Task, Epic)"
    },
    {
      "key": "sprint",
      "displayName": "Sprint",
      "description": "Agile sprint iteration for time-boxed delivery"
    },
    {
      "key": "story_points",
      "displayName": "Story Points",
      "description": "Effort estimation using Fibonacci scale",
      "unit": "points",
      "defaultAggregation": "avg"
    }
  ],
  "suggestedHierarchies": [
    {"parent": "issue_type", "child": "status", "reason": "Bugs and Stories often have different workflow states"}
  ],
  "recoverColumns": [
    {"column": "Summary", "reason": "Issue titles are useful for drill-down views", "suggestedRole": "dimension"}
  ]
}`
}

// ============================================================================
// 1. PAYLOAD BUILDER TESTS
// ============================================================================

func TestBuildRefinePayload(t *testing.T) {
	draft := jiraDraftSchema()
	payload := buildRefinePayload(draft)

	// Should have 4 dimensions + 1 non-synthetic measure = 5 columns
	if len(payload.Columns) != 5 {
		t.Errorf("Expected 5 columns in payload, got %d", len(payload.Columns))
	}

	// Verify synthetic record_count is excluded
	for _, col := range payload.Columns {
		if col.Key == "record_count" {
			t.Error("Synthetic record_count should not be sent to AI")
		}
	}

	// Verify temporal flag propagates
	sprintFound := false
	for _, col := range payload.Columns {
		if col.Key == "sprint" {
			sprintFound = true
			if !col.IsTemporal {
				t.Error("Sprint should be flagged as temporal")
			}
			if col.Type != "temporal" {
				t.Errorf("Sprint type should be 'temporal', got '%s'", col.Type)
			}
		}
	}
	if !sprintFound {
		t.Error("Sprint dimension missing from payload")
	}

	// Verify detected patterns
	if !payload.Detected.HasTemporal {
		t.Error("Detected.HasTemporal should be true")
	}
	if len(payload.Detected.SkippedColumns) != 1 || payload.Detected.SkippedColumns[0] != "Summary" {
		t.Errorf("Expected skipped column 'Summary', got %v", payload.Detected.SkippedColumns)
	}
}

func TestBuildRefinePayloadSampleLimit(t *testing.T) {
	draft := &Config{
		Dimensions: []DimensionMeta{
			{
				Key:          "big_dim",
				DisplayName:  "Big Dimension",
				SampleValues: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			},
		},
	}

	payload := buildRefinePayload(draft)

	// Samples should be limited to 5 in the payload
	if len(payload.Columns) != 1 {
		t.Fatalf("Expected 1 column, got %d", len(payload.Columns))
	}
	if len(payload.Columns[0].Samples) != 5 {
		t.Errorf("Expected 5 sample values (capped), got %d", len(payload.Columns[0].Samples))
	}
}

func TestBuildRefinePayloadWithCurrency(t *testing.T) {
	draft := &Config{
		Dimensions: []DimensionMeta{
			{Key: "currency", DisplayName: "Currency", IsCurrencyCode: true, SampleValues: []string{"SGD", "INR"}},
		},
		Currency: &CurrencyConfig{Enabled: true, CodeDimension: "currency", BaseCurrency: "SGD"},
	}

	payload := buildRefinePayload(draft)
	if !payload.Detected.HasCurrency {
		t.Error("Detected.HasCurrency should be true")
	}
	if !payload.Columns[0].IsCurrencyCode {
		t.Error("Currency column should have IsCurrencyCode flag")
	}
}

// ============================================================================
// 2. RESPONSE PARSER TESTS
// ============================================================================

func TestParseRefineResponseValid(t *testing.T) {
	result, err := parseRefineResponse(mockGeminiResponse())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.DatasetName != "Jira Project Tracker" {
		t.Errorf("Expected dataset name 'Jira Project Tracker', got '%s'", result.DatasetName)
	}

	if len(result.Enrichments) != 5 {
		t.Errorf("Expected 5 enrichments, got %d", len(result.Enrichments))
	}

	// Check story_points enrichment
	for _, e := range result.Enrichments {
		if e.Key == "story_points" {
			if e.Unit != "points" {
				t.Errorf("Expected unit 'points', got '%s'", e.Unit)
			}
			if e.DefaultAggregation != "avg" {
				t.Errorf("Expected defaultAggregation 'avg', got '%s'", e.DefaultAggregation)
			}
		}
	}

	if len(result.SuggestedHierarchies) != 1 {
		t.Errorf("Expected 1 hierarchy suggestion, got %d", len(result.SuggestedHierarchies))
	}

	if len(result.RecoverColumns) != 1 {
		t.Errorf("Expected 1 recovery suggestion, got %d", len(result.RecoverColumns))
	}
}

func TestParseRefineResponseWithMarkdown(t *testing.T) {
	// Gemini sometimes wraps in ```json ... ```
	wrapped := "```json\n" + mockGeminiResponse() + "\n```"
	result, err := parseRefineResponse(wrapped)
	if err != nil {
		t.Fatalf("Parse should handle markdown wrapping: %v", err)
	}
	if result.DatasetName != "Jira Project Tracker" {
		t.Errorf("Expected 'Jira Project Tracker', got '%s'", result.DatasetName)
	}
}

func TestParseRefineResponseInvalidJSON(t *testing.T) {
	_, err := parseRefineResponse("this is not json")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseRefineResponseEmptyEnrichments(t *testing.T) {
	response := `{"datasetName": "Test", "datasetDescription": "Test data", "enrichments": [], "suggestedHierarchies": [], "recoverColumns": []}`
	result, err := parseRefineResponse(response)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if result.DatasetName != "Test" {
		t.Errorf("Expected 'Test', got '%s'", result.DatasetName)
	}
	if len(result.Enrichments) != 0 {
		t.Errorf("Expected 0 enrichments, got %d", len(result.Enrichments))
	}
}

// ============================================================================
// 3. ENRICHMENT APPLICATOR TESTS
// ============================================================================

func TestApplyEnrichments(t *testing.T) {
	draft := jiraDraftSchema()
	enrichment, _ := parseRefineResponse(mockGeminiResponse())

	result := applyEnrichments(draft, enrichment)

	// Dataset-level
	if result.Name != "Jira Project Tracker" {
		t.Errorf("Expected name 'Jira Project Tracker', got '%s'", result.Name)
	}
	if result.Description != "Issue tracking data from a Jira project board" {
		t.Errorf("Expected description mismatch, got '%s'", result.Description)
	}

	// Dimension enrichments
	for _, d := range result.Dimensions {
		switch d.Key {
		case "status":
			if d.Description != "Issue workflow state in the Kanban board" {
				t.Errorf("Status description not enriched: %s", d.Description)
			}
			if d.SortHint != "To Do > In Progress > Review > Done" {
				t.Errorf("Status sortHint not set: %s", d.SortHint)
			}
		case "priority":
			if !strings.Contains(d.SortHint, "P1") {
				t.Errorf("Priority sortHint not set: %s", d.SortHint)
			}
		}
	}

	// Measure enrichments
	for _, m := range result.Measures {
		if m.Key == "story_points" {
			if m.Unit != "points" {
				t.Errorf("Story points unit not enriched: %s", m.Unit)
			}
			if m.DefaultAggregation != "avg" {
				t.Errorf("Story points defaultAgg not changed to avg: %s", m.DefaultAggregation)
			}
			if m.Description != "Effort estimation using Fibonacci scale" {
				t.Errorf("Story points description not enriched: %s", m.Description)
			}
		}
	}

	// Hierarchy suggestion applied (status → issue_type, only if not already set)
	statusHasParent := false
	for _, d := range result.Dimensions {
		if d.Key == "status" && d.Parent == "issue_type" {
			statusHasParent = true
		}
	}
	if !statusHasParent {
		t.Error("Hierarchy suggestion (status → issue_type) not applied")
	}

	// RefinedAt should be set
	if result.RefinedAt == "" {
		t.Error("RefinedAt should be set after enrichment")
	}
	if result.RefinedBy != "gemini" {
		t.Errorf("RefinedBy should be 'gemini', got '%s'", result.RefinedBy)
	}
}

func TestApplyEnrichmentDoesNotOverrideExistingParent(t *testing.T) {
	draft := &Config{
		Dimensions: []DimensionMeta{
			{Key: "child", DisplayName: "Child", Parent: "existing_parent"},
			{Key: "existing_parent", DisplayName: "Existing Parent"},
			{Key: "suggested_parent", DisplayName: "Suggested Parent"},
		},
	}

	enrichment := &refineEnrichment{
		SuggestedHierarchies: []hierarchySuggestion{
			{Parent: "suggested_parent", Child: "child"},
		},
	}

	result := applyEnrichments(draft, enrichment)

	for _, d := range result.Dimensions {
		if d.Key == "child" {
			if d.Parent != "existing_parent" {
				t.Errorf("Existing parent should NOT be overridden, got '%s'", d.Parent)
			}
		}
	}
}

func TestApplyEnrichmentInvalidAggregation(t *testing.T) {
	draft := &Config{
		Measures: []MeasureMeta{
			{Key: "amount", DisplayName: "Amount", DefaultAggregation: "sum"},
		},
	}

	enrichment := &refineEnrichment{
		Enrichments: []columnEnrichment{
			{Key: "amount", DefaultAggregation: "median"}, // invalid
		},
	}

	result := applyEnrichments(draft, enrichment)

	if result.Measures[0].DefaultAggregation != "sum" {
		t.Errorf("Invalid aggregation 'median' should not override, got '%s'", result.Measures[0].DefaultAggregation)
	}
}

func TestApplyEnrichmentCurrencyUnit(t *testing.T) {
	draft := &Config{
		Measures: []MeasureMeta{
			{Key: "revenue", DisplayName: "Revenue"},
		},
	}

	enrichment := &refineEnrichment{
		Enrichments: []columnEnrichment{
			{Key: "revenue", Unit: "currency"},
		},
	}

	result := applyEnrichments(draft, enrichment)

	if !result.Measures[0].IsCurrency {
		t.Error("IsCurrency should be set when unit is 'currency'")
	}
}

// ============================================================================
// 4. DEEP COPY ISOLATION TESTS
// ============================================================================

func TestDeepCopyIsolation(t *testing.T) {
	draft := jiraDraftSchema()
	enrichment, _ := parseRefineResponse(mockGeminiResponse())

	// Keep a reference to original values
	origName := draft.Name
	origStatusDesc := draft.Dimensions[0].Description
	origStoryPointsAgg := draft.Measures[0].DefaultAggregation

	_ = applyEnrichments(draft, enrichment)

	// Verify draft is NOT mutated
	if draft.Name != origName {
		t.Errorf("Draft name was mutated: '%s' → '%s'", origName, draft.Name)
	}
	if draft.Dimensions[0].Description != origStatusDesc {
		t.Errorf("Draft dimension description was mutated")
	}
	if draft.Measures[0].DefaultAggregation != origStoryPointsAgg {
		t.Errorf("Draft measure aggregation was mutated")
	}
}

func TestDeepCopySliceIsolation(t *testing.T) {
	draft := jiraDraftSchema()
	result := deepCopyConfig(draft)

	// Mutate the copy's sample values
	result.Dimensions[0].SampleValues[0] = "MUTATED"

	// Original should be unchanged
	if draft.Dimensions[0].SampleValues[0] == "MUTATED" {
		t.Error("Deep copy did not isolate SampleValues slice")
	}
}

func TestDeepCopyCurrencyIsolation(t *testing.T) {
	draft := &Config{
		Currency: &CurrencyConfig{
			Enabled:      true,
			BaseCurrency: "SGD",
			Rates:        map[string]float64{"INR": 62.5, "USD": 0.74},
		},
	}

	result := deepCopyConfig(draft)
	result.Currency.Rates["EUR"] = 0.68

	if _, ok := draft.Currency.Rates["EUR"]; ok {
		t.Error("Deep copy did not isolate Currency.Rates map")
	}
}

// ============================================================================
// 5. EDGE CASE TESTS
// ============================================================================

func TestRefineNilConfig(t *testing.T) {
	_, err := Refine(nil, DefaultRefineConfig("test-key"))
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

func TestRefineEmptyAPIKey(t *testing.T) {
	draft := jiraDraftSchema()
	_, err := Refine(draft, RefineConfig{})
	if err == nil {
		t.Error("Expected error for empty API key")
	}
}

func TestBuildRefinePayloadEmptySchema(t *testing.T) {
	draft := &Config{}
	payload := buildRefinePayload(draft)

	if len(payload.Columns) != 0 {
		t.Errorf("Expected 0 columns for empty schema, got %d", len(payload.Columns))
	}
}

func TestPayloadSerializationSize(t *testing.T) {
	draft := jiraDraftSchema()
	payload := buildRefinePayload(draft)
	payloadJSON, _ := json.Marshal(payload)

	// Payload should be lightweight — under 2KB for a typical schema
	if len(payloadJSON) > 2048 {
		t.Errorf("Payload too large: %d bytes (expected < 2048)", len(payloadJSON))
	}

	t.Logf("📊 Payload size: %d bytes for %d columns", len(payloadJSON), len(payload.Columns))
}

// ============================================================================
// HELPERS
// ============================================================================

func TestIsValidAggregation(t *testing.T) {
	valid := []string{"sum", "avg", "count", "max", "min"}
	for _, agg := range valid {
		if !isValidAggregation(agg) {
			t.Errorf("Expected '%s' to be valid", agg)
		}
	}

	invalid := []string{"median", "mode", "stdev", "", "SUM"}
	for _, agg := range invalid {
		if isValidAggregation(agg) {
			t.Errorf("Expected '%s' to be invalid", agg)
		}
	}
}

func TestLimitSamples(t *testing.T) {
	vals := []string{"a", "b", "c", "d", "e", "f"}
	result := limitSamples(vals, 3)
	if len(result) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(result))
	}

	// Under limit — return all
	result2 := limitSamples(vals, 10)
	if len(result2) != 6 {
		t.Errorf("Expected 6 samples (all), got %d", len(result2))
	}
}

func TestEstimateUnique(t *testing.T) {
	if estimateUnique("low") != 5 {
		t.Error("low should return 5")
	}
	if estimateUnique("medium") != 30 {
		t.Error("medium should return 30")
	}
	if estimateUnique("high") != 200 {
		t.Error("high should return 200")
	}
	if estimateUnique("unknown") != 10 {
		t.Error("unknown should return 10")
	}
}