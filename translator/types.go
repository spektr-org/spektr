package translator

import (
	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// TRANSLATOR — AI boundary for natural language → QuerySpec
// ============================================================================
// The translator is the ONLY component that calls an external AI service.
// It receives schema metadata + user question, returns a QuerySpec.
// It NEVER sees raw data — only column names, sample values, and the question.
// ============================================================================

// AIProvider is the single integration point for any AI provider.
// Consumers implement this interface for their provider of choice.
//
// Spektr calls Complete with a fully-formed prompt string and expects
// the raw text response back. All HTTP transport, authentication, and
// request/response shaping for the specific provider is the consumer's concern.
//
// A reference implementation for Gemini is in translator/adapters/gemini.go.
// TPL consumers route through their relay by implementing this interface.
//
// Example custom adapter:
//
//	type MyRelayAdapter struct { relayURL string }
//
//	func (a *MyRelayAdapter) Complete(prompt string) (string, error) {
//	    resp, err := http.Post(a.relayURL, "application/json",
//	        strings.NewReader(`{"prompt":` + strconv.Quote(prompt) + `}`))
//	    // ... parse and return text response
//	}
type AIProvider interface {
	Complete(prompt string) (string, error)
}

// Translator translates natural language queries into QuerySpecs.
type Translator interface {
	Translate(query string, sch schema.Config) (*TranslateResult, error)
	TranslateWithSummary(query string, sch schema.Config, summary *DataSummary) (*TranslateResult, error)
}

// TranslateResult contains the QuerySpec and the AI's interpretation.
type TranslateResult struct {
	QuerySpec      engine.QuerySpec      `json:"querySpec"`
	Interpretation engine.Interpretation `json:"interpretation"`
}

// Config holds AI provider configuration.
// Consumed by the built-in adapters — custom adapters manage their own config.
type Config struct {
	APIKey   string // Consumer's API key
	Model    string // Model name (e.g. "gemini-2.5-flash-lite", "gpt-4o")
	Endpoint string // Provider endpoint — leave empty for adapter default
}