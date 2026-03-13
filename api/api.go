package api

// ============================================================================
// SPEKTR PUBLIC API — Interface Functions
// ============================================================================
// Each function here is the Go implementation of one REST endpoint.
//
// Consumers call these functions directly (Go, WASM) or through a thin
// transport wrapper (Lambda handler, HTTP mux, Apps Script relay).
//
// Contract:
//   - Functions never return Go errors — errors are inside the Response envelope.
//   - Functions are stateless — no shared mutable state.
//   - Functions are safe to call concurrently.
//   - The AI functions (Refine, Translate) make outbound network calls.
//     All other functions are pure local computation.
// ============================================================================

import (
	"fmt"
	"strings"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/helpers"
	"github.com/spektr-org/spektr/schema"
	"github.com/spektr-org/spektr/translator"
	"github.com/spektr-org/spektr/translator/adapters"
)

// Version is the current Spektr library version.
const Version = "1.0.0"

// ============================================================================
// Health
// ============================================================================

// Health returns the engine version and readiness status.
// Maps to: GET /health
//
// Example:
//
//	result := api.Health()
//	// result.Data.Version == "1.0.0"
//	// result.Data.Status  == "ready"
func Health() Response[HealthResult] {
	return ok(HealthResult{
		Version: Version,
		Status:  "ready",
	})
}

// ============================================================================
// Discover
// ============================================================================

// Discover inspects a raw CSV and automatically classifies each column as
// a dimension (string, groupable) or measure (numeric, aggregatable).
// Returns a SchemaConfig describing the dataset structure.
// Maps to: POST /discover
//
// The schema can be cached and reused for the same dataset shape.
// Call Refine once to AI-enrich the schema; then reuse the refined schema.
//
// Example:
//
//	resp := api.Discover(api.DiscoverRequest{
//	    CSV:  csvContent,
//	    Name: "Playbook Run Analysis",
//	})
//	if !resp.OK {
//	    // handle resp.Error
//	}
//	schema := resp.Data
func Discover(req DiscoverRequest) DiscoverResponse {
	if strings.TrimSpace(req.CSV) == "" {
		return fail[schema.Config]("csv is required")
	}

	opts := schema.DiscoverOptions{
		Name:           req.Name,
		SampleSize:     req.SampleSize,
		RecoverColumns: req.RecoverColumns,
	}

	result, err := schema.DiscoverFromCSV([]byte(req.CSV), opts)
	if err != nil {
		return fail[schema.Config](fmt.Sprintf("discover failed: %s", err.Error()))
	}

	return ok(*result)
}

// ============================================================================
// Refine
// ============================================================================

// Refine enriches a discovered schema using a one-time AI call.
// Adds human-readable descriptions, corrects misclassifications, and detects
// semantic meaning (temporal fields, ordinal orderings, domain-specific units).
// Maps to: POST /refine
//
// Call this once per schema and cache the result. Do not call on every request —
// it makes an outbound AI call and consumes API tokens.
// The input schema is never mutated; a new enriched schema is returned.
//
// Example:
//
//	resp := api.Refine(api.RefineRequest{
//	    Schema: discoveredSchema,
//	    APIKey: "AIza...",
//	    Model:  "gemini-2.5-flash-lite",  // optional
//	})
//	if !resp.OK {
//	    // handle resp.Error — original schema still usable
//	}
//	refinedSchema := resp.Data
func Refine(req RefineRequest) RefineResponse {
	if req.APIKey == "" {
		return fail[schema.Config]("apiKey is required for refine")
	}

	cfg := schema.DefaultRefineConfig(req.APIKey)
	if req.Model != "" {
		cfg.Model = req.Model
	}
	if req.Endpoint != "" {
		cfg.Endpoint = req.Endpoint
	}

	result, err := schema.Refine(&req.Schema, cfg)
	if err != nil {
		return fail[schema.Config](fmt.Sprintf("refine failed: %s", err.Error()))
	}

	return ok(*result)
}

// ============================================================================
// Parse
// ============================================================================

// Parse converts a raw CSV into structured records using a schema.
// Each row becomes a Record with dimensions (strings) and measures (numbers).
// The returned records are the input to Execute.
// Maps to: POST /parse
//
// Example:
//
//	resp := api.Parse(api.ParseRequest{
//	    CSV:    csvContent,
//	    Schema: schema,
//	})
//	if !resp.OK {
//	    // handle resp.Error
//	}
//	records := resp.Data.Records   // pass to Execute
//	count   := resp.Data.Count
func Parse(req ParseRequest) ParseResponse {
	if strings.TrimSpace(req.CSV) == "" {
		return fail[ParseResult]("csv is required")
	}
	if len(req.Schema.Dimensions) == 0 && len(req.Schema.Measures) == 0 {
		return fail[ParseResult]("schema must have at least one dimension or measure")
	}

	records, err := helpers.ParseCSV([]byte(req.CSV), req.Schema)
	if err != nil {
		return fail[ParseResult](fmt.Sprintf("parse failed: %s", err.Error()))
	}

	return ok(ParseResult{
		Records: records,
		Count:   len(records),
	})
}

// ============================================================================
// Translate
// ============================================================================

// Translate converts a natural language query into a QuerySpec using an AI model.
// The QuerySpec can be passed directly to Execute.
// Maps to: POST /translate
//
// The summary provides lightweight context to the AI without exposing raw data —
// only record counts and sample dimension values are sent to the AI provider.
// Build the summary with SummaryFromRecords().
//
// Example:
//
//	summary := api.SummaryFromRecords(records, schema)
//	resp := api.Translate(api.TranslateRequest{
//	    Query:   "which playbooks have the highest failure rate",
//	    Schema:  schema,
//	    Summary: summary,
//	    APIKey:  "AIza...",
//	})
//	if !resp.OK {
//	    // handle resp.Error
//	}
//	spec := resp.Data.QuerySpec   // pass to Execute
func Translate(req TranslateRequest) TranslateResponse {
	if strings.TrimSpace(req.Query) == "" {
		return fail[TranslateResult]("query is required")
	}
	if req.APIKey == "" {
		return fail[TranslateResult]("apiKey is required for translate")
	}
	if len(req.Schema.Dimensions) == 0 && len(req.Schema.Measures) == 0 {
		return fail[TranslateResult]("schema must have at least one dimension or measure")
	}

	adapter := adapters.New(req.APIKey, req.Model, req.Endpoint)
	t := translator.NewTranslator(adapter)

	summary := translator.DataSummary{
		RecordCount: req.Summary.RecordCount,
		Dimensions:  req.Summary.Dimensions,
	}

	result, err := t.TranslateWithSummary(req.Query, req.Schema, &summary)
	if err != nil {
		return fail[TranslateResult](fmt.Sprintf("translate failed: %s", err.Error()))
	}

	return ok(TranslateResult{
		QuerySpec:      result.QuerySpec,
		Interpretation: result.Interpretation,
	})
}

// ============================================================================
// Execute
// ============================================================================

// Execute runs a QuerySpec against a set of records.
// Pure local computation — no AI, no network calls.
// Maps to: POST /execute
//
// The result contains chartConfig (for rendering charts), tableData
// (for rendering tables), and reply (human-readable summary sentence).
// The consumer chooses which representation to use.
//
// Example:
//
//	resp := api.Execute(api.ExecuteRequest{
//	    Spec:    querySpec,
//	    Records: records,
//	})
//	if !resp.OK {
//	    // handle resp.Error
//	}
//	result := resp.Data
//	// result.ChartConfig → render a chart
//	// result.TableData   → render a table
//	// result.Reply       → display as text summary
func Execute(req ExecuteRequest) ExecuteResponse {
	if len(req.Records) == 0 {
		return fail[engine.Result]("records is required and must not be empty")
	}

	opts := []engine.Option{}
	if req.Options != nil {
		if req.Options.DefaultMeasure != "" {
			opts = append(opts, engine.WithDefaultMeasure(req.Options.DefaultMeasure))
		}
		if req.Options.BaseCurrency != "" && len(req.Options.ExchangeRates) > 0 {
			dim := req.Options.CurrencyDimension
			if dim == "" {
				dim = "currency"
			}
			opts = append(opts, engine.WithCurrency(req.Options.BaseCurrency, dim, req.Options.ExchangeRates))
		}
	}

	view := engine.NewSliceView(req.Records)
	result, err := engine.Execute(req.Spec, view, opts...)
	if err != nil {
		return fail[engine.Result](fmt.Sprintf("execute failed: %s", err.Error()))
	}

	return ok(*result)
}

// ============================================================================
// Pipeline
// ============================================================================

// Pipeline composes Discover → Parse → (Translate) → Execute in one call.
// Designed for stateless consumers who want results in one round-trip.
// Maps to: POST /pipeline
//
// Use the individual functions when caching schema or records between queries.
// Use Pipeline for one-off or stateless requests (scripts, automation, Lambda).
//
// Example (local / keyword mode):
//
//	resp := api.Pipeline(api.PipelineRequest{
//	    CSV:   csvContent,
//	    Query: "count records by playbook_id",
//	    Mode:  api.PipelineModeLocal,
//	})
//
// Example (AI / natural language mode):
//
//	resp := api.Pipeline(api.PipelineRequest{
//	    CSV:    csvContent,
//	    Query:  "which playbooks have the highest failure rate",
//	    Mode:   api.PipelineModeAI,
//	    APIKey: "AIza...",
//	})
func Pipeline(req PipelineRequest) PipelineResponse {
	if strings.TrimSpace(req.CSV) == "" {
		return fail[PipelineResult]("csv is required")
	}
	if strings.TrimSpace(req.Query) == "" {
		return fail[PipelineResult]("query is required")
	}
	if req.Mode == PipelineModeAI && req.APIKey == "" {
		return fail[PipelineResult]("apiKey is required when mode is 'ai'")
	}

	// Step 1: Discover schema (skip if provided)
	var sch schema.Config
	if req.Schema != nil {
		sch = *req.Schema
	} else {
		discoverResp := Discover(DiscoverRequest{CSV: req.CSV})
		if !discoverResp.OK {
			return fail[PipelineResult](fmt.Sprintf("discover step failed: %s", discoverResp.Error))
		}
		sch = *discoverResp.Data
	}

	// Step 2: Parse records
	parseResp := Parse(ParseRequest{CSV: req.CSV, Schema: sch})
	if !parseResp.OK {
		return fail[PipelineResult](fmt.Sprintf("parse step failed: %s", parseResp.Error))
	}
	records := parseResp.Data.Records

	// Step 3: Translate query → QuerySpec
	var spec engine.QuerySpec
	mode := req.Mode
	if mode == "" {
		mode = PipelineModeLocal
	}

	if mode == PipelineModeAI {
		summary := SummaryFromRecords(records, sch)
		translateResp := Translate(TranslateRequest{
			Query:   req.Query,
			Schema:  sch,
			Summary: DataSummary{RecordCount: summary.RecordCount, Dimensions: summary.Dimensions},
			APIKey:  req.APIKey,
			Model:   req.Model,
		})
		if !translateResp.OK {
			return fail[PipelineResult](fmt.Sprintf("translate step failed: %s", translateResp.Error))
		}
		spec = translateResp.Data.QuerySpec
	} else {
		// Local keyword mode: build a basic QuerySpec from the query string.
		// Supports: "sum <measure> by <dimension>", "count records by <dimension>",
		// "avg <measure> by <dimension>". For NL queries use PipelineModeAI.
		spec = buildLocalSpec(req.Query, sch)
	}

	// Step 4: Execute
	executeResp := Execute(ExecuteRequest{
		Spec:    spec,
		Records: records,
	})
	if !executeResp.OK {
		return fail[PipelineResult](fmt.Sprintf("execute step failed: %s", executeResp.Error))
	}

	return ok(PipelineResult{
		Schema:      sch,
		RecordCount: parseResp.Data.Count,
		Query:       req.Query,
		Spec:        spec,
		Result:      *executeResp.Data,
	})
}

// ============================================================================
// HELPERS
// ============================================================================

// SummaryFromRecords builds a translator.DataSummary from parsed records.
// Only dimension sample values and record count are included.
// Raw measure values are never sent to the AI.
func SummaryFromRecords(records []engine.Record, sch schema.Config) translator.DataSummary {
	return *translator.BuildDataSummaryFromRecords(records, sch)
}
// buildLocalSpec constructs a basic QuerySpec from a plain query string.
// Supports simple patterns: "sum <measure> by <dimension>",
// "count records by <dimension>", "avg <measure> by <dimension>".
// For natural language queries use PipelineModeAI.
func buildLocalSpec(query string, sch schema.Config) engine.QuerySpec {
	lower := strings.ToLower(strings.TrimSpace(query))

	spec := engine.QuerySpec{
		Intent:      "chart",
		Aggregation: "sum",
		Visualize:   "bar",
		SortBy:      "value_desc",
		Limit:       20,
		Confidence:  1.0,
	}

	// Default measure — first non-synthetic measure in schema
	for _, m := range sch.Measures {
		if !m.IsSynthetic {
			spec.Measure = m.Key
			break
		}
	}
	if spec.Measure == "" && len(sch.Measures) > 0 {
		spec.Measure = sch.Measures[0].Key
	}

	// Parse aggregation keyword
	for _, agg := range []string{"sum", "avg", "min", "max", "count"} {
		if strings.Contains(lower, agg) {
			spec.Aggregation = agg
			break
		}
	}

	// Match measure key or display name from query
	for _, m := range sch.Measures {
		if strings.Contains(lower, m.Key) || strings.Contains(lower, strings.ToLower(m.DisplayName)) {
			spec.Measure = m.Key
			break
		}
	}

	// Parse groupBy — look for "by <dimension>"
	if idx := strings.Index(lower, " by "); idx != -1 {
		afterBy := strings.TrimSpace(lower[idx+4:])
		for _, d := range sch.Dimensions {
			if strings.HasPrefix(afterBy, d.Key) || strings.HasPrefix(afterBy, strings.ToLower(d.DisplayName)) {
				spec.GroupBy = []string{d.Key}
				break
			}
		}
	}

	// Default groupBy — first dimension if nothing matched
	if len(spec.GroupBy) == 0 && len(sch.Dimensions) > 0 {
		spec.GroupBy = []string{sch.Dimensions[0].Key}
	}

	spec.Title = fmt.Sprintf("%s %s by %s",
		strings.ToUpper(spec.Aggregation[:1])+spec.Aggregation[1:],
		spec.Measure,
		strings.Join(spec.GroupBy, ", "))

	return spec
}