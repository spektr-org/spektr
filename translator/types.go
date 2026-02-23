package translator

import (
	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// TRANSLATOR — AI boundary for natural language → QuerySpec
// ============================================================================
// The Translator is the ONLY component that calls an external AI service.
// It receives schema metadata + user question, returns a QuerySpec.
// It NEVER sees raw data. Only column names, sample values, and the question.
//
// TPL origin: handlers/nlm.go (buildQuerySpecPrompt + parseQuerySpecResponse)
// Key change: Hardcoded finance prompt → schema-driven prompt builder.
// ============================================================================

// Translator translates natural language queries into QuerySpecs.
// Implementations: Gemini (v1), OpenAI (future), local LLM (future).
type Translator interface {
	// Translate converts a natural language query into a QuerySpec.
	// The schema provides metadata for prompt building.
	// Returns both the QuerySpec (for engine) and Interpretation (for user preview).
	Translate(query string, sch schema.Config) (*TranslateResult, error)
}

// TranslateResult contains both the QuerySpec and the Interpretation.
// Mirrors TPL's two-phase flow: interpret (preview) → execute (compute).
type TranslateResult struct {
	QuerySpec      engine.QuerySpec      `json:"querySpec"`
	Interpretation engine.Interpretation `json:"interpretation"`
}

// Config holds translator configuration.
type Config struct {
	APIKey   string // AI provider API key (consumer's key)
	Model    string // Model name (e.g., "gemini-2.0-flash")
	Endpoint string // API endpoint override (empty = default)
}

// DefaultGeminiConfig returns a Config with sensible Gemini defaults.
func DefaultGeminiConfig(apiKey string) Config {
	return Config{
		APIKey:   apiKey,
		Model:    "gemini-2.0-flash",
		Endpoint: "https://generativelanguage.googleapis.com/v1beta/models",
	}
}
