package api

// ============================================================================
// SPEKTR PUBLIC API — Canonical Request / Response Types
// ============================================================================
// Every function in this package maps to one REST endpoint by design.
// Consumers (Lambda handlers, HTTP wrappers, WASM bridges, Apps Script) use
// these types as the contract — they JSON-encode a Request, call the function,
// and JSON-decode the Response.
//
// Transport is never Spektr's concern. The library defines the contract;
// the consumer chooses how to expose it.
//
// Envelope rule:
//   - ok=true  → data is populated, error is empty
//   - ok=false → error is populated, data is nil/zero
//
// All Request and Response types are safe to JSON-marshal/unmarshal directly.
// ============================================================================

import (
	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// SHARED ENVELOPE
// ============================================================================

// Response is the generic envelope for all Spektr API responses.
// T is the data payload type for each endpoint.
type Response[T any] struct {
	OK    bool   `json:"ok"`
	Data  *T     `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// ok constructs a successful response.
func ok[T any](data T) Response[T] {
	return Response[T]{OK: true, Data: &data}
}

// fail constructs an error response.
func fail[T any](err string) Response[T] {
	return Response[T]{OK: false, Error: err}
}

// ============================================================================
// /health
// ============================================================================

// HealthResult is the response payload for the health endpoint.
type HealthResult struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// ============================================================================
// /discover
// ============================================================================

// DiscoverRequest is the input for the Discover function.
// CSV is the raw CSV text. All other fields are optional.
type DiscoverRequest struct {
	// CSV is the raw CSV content to analyse. Required.
	CSV string `json:"csv"`

	// Name overrides the auto-inferred dataset name.
	Name string `json:"name,omitempty"`

	// SampleSize limits how many rows are inspected during discovery.
	// Defaults to 1000. Set to 0 to inspect all rows.
	SampleSize int `json:"sampleSize,omitempty"`

	// RecoverColumns is a list of column names that were auto-skipped
	// but should be force-included as dimensions.
	RecoverColumns []string `json:"recoverColumns,omitempty"`
}

// DiscoverResponse is the output of the Discover function.
type DiscoverResponse = Response[schema.Config]

// ============================================================================
// /refine
// ============================================================================

// RefineRequest is the input for the Refine function.
// Refine should be called once per schema and the result cached.
// It makes an outbound AI call — do not call on every request.
type RefineRequest struct {
	// Schema is the output from Discover. Required.
	Schema schema.Config `json:"schema"`

	// APIKey is the consumer's AI provider key. Required.
	APIKey string `json:"apiKey"`

	// Model overrides the default AI model.
	// Defaults to "gemini-2.5-flash-lite".
	Model string `json:"model,omitempty"`

	// Endpoint overrides the default AI provider endpoint.
	// Defaults to the Gemini v1beta endpoint.
	Endpoint string `json:"endpoint,omitempty"`
}

// RefineResponse is the output of the Refine function.
// The returned schema is an enriched copy — the input schema is never mutated.
type RefineResponse = Response[schema.Config]

// ============================================================================
// /parse
// ============================================================================

// ParseRequest is the input for the Parse function.
type ParseRequest struct {
	// CSV is the raw CSV content to parse. Required.
	CSV string `json:"csv"`

	// Schema describes how to classify columns. Required.
	// Typically the output from Discover or Refine.
	Schema schema.Config `json:"schema"`
}

// ParseResult is the data payload returned by Parse.
type ParseResult struct {
	// Records is the parsed dataset ready for Execute.
	Records []engine.Record `json:"records"`

	// Count is the number of records parsed.
	Count int `json:"count"`
}

// ParseResponse is the output of the Parse function.
type ParseResponse = Response[ParseResult]

// ============================================================================
// /translate
// ============================================================================

// DataSummary is a lightweight description of a parsed dataset.
// It provides the AI with enough context to understand the data
// without ever exposing raw values — only counts and sample dimension values.
//
// Build this from parsed records:
//
//	summary := api.SummaryFromRecords(records, schema)
type DataSummary struct {
	// RecordCount is the total number of records in the dataset.
	RecordCount int `json:"recordCount"`

	// Dimensions maps each dimension key to a sample of its values (max 8).
	// These samples help the AI understand what values are filterable.
	Dimensions map[string][]string `json:"dimensions"`
}

// TranslateRequest is the input for the Translate function.
// Translate makes an outbound AI call to convert natural language into a QuerySpec.
type TranslateRequest struct {
	// Query is the natural language question. Required.
	// Examples: "which playbooks have the highest failure rate",
	//           "show total runs by playbook last week"
	Query string `json:"query"`

	// Schema describes the dataset structure. Required.
	Schema schema.Config `json:"schema"`

	// Summary provides lightweight dataset context to the AI. Required.
	// Build with api.SummaryFromRecords().
	Summary DataSummary `json:"summary"`

	// APIKey is the consumer's AI provider key. Required.
	APIKey string `json:"apiKey"`

	// Model overrides the default AI model.
	Model string `json:"model,omitempty"`
	// Endpoint overrides the default AI provider endpoint.
	// Leave empty to use the Gemini default.
	Endpoint string `json:"endpoint,omitempty"`
}

// TranslateResult is the data payload returned by Translate.
type TranslateResult struct {
	// QuerySpec is the structured query ready to pass to Execute.
	QuerySpec engine.QuerySpec `json:"querySpec"`

	// Interpretation describes what the AI understood.
	Interpretation engine.Interpretation `json:"interpretation"`
}

// TranslateResponse is the output of the Translate function.
type TranslateResponse = Response[TranslateResult]

// ============================================================================
// /execute
// ============================================================================

// ExecuteOptions configures optional engine behaviours.
// All fields are optional — omit to use engine defaults.
type ExecuteOptions struct {
	// DefaultMeasure is used when the QuerySpec does not specify a measure.
	DefaultMeasure string `json:"defaultMeasure,omitempty"`

	// BaseCurrency is the target currency for conversion (e.g. "SGD").
	// Requires CurrencyDimension and ExchangeRates to be set.
	BaseCurrency string `json:"baseCurrency,omitempty"`

	// CurrencyDimension is the dimension key holding currency codes.
	CurrencyDimension string `json:"currencyDimension,omitempty"`

	// ExchangeRates maps currency codes to their rate relative to BaseCurrency.
	ExchangeRates map[string]float64 `json:"exchangeRates,omitempty"`
}

// ExecuteRequest is the input for the Execute function.
// Execute is pure local computation — no AI, no network calls.
type ExecuteRequest struct {
	// Spec is the query to execute. Required.
	// Either hand-constructed or the output of Translate.
	Spec engine.QuerySpec `json:"spec"`

	// Records is the parsed dataset to run the query against. Required.
	// The output of Parse.
	Records []engine.Record `json:"records"`

	// Options configures optional behaviours like currency conversion.
	Options *ExecuteOptions `json:"options,omitempty"`
}

// ExecuteResponse is the output of the Execute function.
type ExecuteResponse = Response[engine.Result]

// ============================================================================
// /pipeline
// ============================================================================

// PipelineMode controls whether natural language translation is used.
type PipelineMode string

const (
	// PipelineModeLocal uses keyword-based query matching. No AI, no API key needed.
	PipelineModeLocal PipelineMode = "local"

	// PipelineModeAI uses the Translate function for natural language queries.
	// Requires APIKey.
	PipelineModeAI PipelineMode = "ai"
)

// PipelineRequest is the input for the Pipeline function.
// Pipeline composes Discover → Parse → (Translate) → Execute in one call.
//
// Use this for stateless / one-shot requests.
// Use the individual functions when caching schema or records between queries.
type PipelineRequest struct {
	// CSV is the raw CSV content. Required.
	CSV string `json:"csv"`

	// Query is the question to answer against the data. Required.
	Query string `json:"query"`

	// Mode controls query translation. Defaults to PipelineModeLocal.
	Mode PipelineMode `json:"mode,omitempty"`

	// APIKey is required when Mode is PipelineModeAI.
	APIKey string `json:"apiKey,omitempty"`

	// Model overrides the default AI model when Mode is PipelineModeAI.
	Model string `json:"model,omitempty"`

	// Schema allows the consumer to supply a pre-built or cached schema,
	// skipping the Discover step. Optional.
	Schema *schema.Config `json:"schema,omitempty"`
}

// PipelineResult is the data payload returned by Pipeline.
type PipelineResult struct {
	// Schema is the schema used for this pipeline run.
	// Either discovered from the CSV or the schema passed in the request.
	Schema schema.Config `json:"schema"`

	// RecordCount is the number of records parsed from the CSV.
	RecordCount int `json:"recordCount"`

	// Query is the original query string from the request.
	Query string `json:"query"`

	// Spec is the QuerySpec that was executed.
	Spec engine.QuerySpec `json:"spec"`

	// Result is the engine output ready for rendering.
	Result engine.Result `json:"result"`
}

// PipelineResponse is the output of the Pipeline function.
type PipelineResponse = Response[PipelineResult]