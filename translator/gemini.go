package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// GEMINI TRANSLATOR — Calls Google Gemini for NL → QuerySpec
// ============================================================================
// TPL origin: handlers/nlm.go (GeminiService.CallSingleTurn + prompt building)
//
// Key changes:
//   - Schema-driven prompt (not hardcoded finance)
//   - No HTTP handler logic (that stays in consumer app)
//   - No rate limiting (consumer's responsibility)
//   - Data summary built from engine.Record (not TPL Transaction)
//
// This is the ONLY file that makes external API calls.
// ============================================================================

// GeminiTranslator implements Translator using Google Gemini API.
type GeminiTranslator struct {
	config  Config
	client  *http.Client
}

// NewGemini creates a new Gemini translator.
func NewGemini(cfg Config) *GeminiTranslator {
	if cfg.Model == "" {
		cfg.Model = "gemini-2.5-flash-lite"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://generativelanguage.googleapis.com/v1beta/models"
	}

	return &GeminiTranslator{
		config: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Translate converts a natural language query into a QuerySpec.
// Takes optional records to build data summary for the prompt.
func (g *GeminiTranslator) Translate(query string, sch schema.Config) (*TranslateResult, error) {
	return g.TranslateWithSummary(query, sch, nil)
}

// TranslateWithSummary converts a query using a pre-built data summary.
// Consumers can build the DataSummary themselves or use BuildDataSummaryFromRecords.
func (g *GeminiTranslator) TranslateWithSummary(query string, sch schema.Config, summary *DataSummary) (*TranslateResult, error) {
	// 1. Build schema-driven prompt
	systemPrompt := BuildPrompt(sch, summary)

	// 2. Annotate ratio queries (same heuristic as TPL)
	annotation := ""
	lowerQuery := strings.ToLower(query)
	ratioKeywords := []string{"percentage of", "% of", "how much of", "portion of", "fraction of", "what part of"}
	for _, kw := range ratioKeywords {
		if strings.Contains(lowerQuery, kw) {
			annotation = "\nHINT: This is a RATIO query. Use aggregation:\"ratio\" with BOTH \"filters\" (denominator) AND \"compareFilters\" (numerator).\n"
			break
		}
	}

	fullPrompt := systemPrompt + "\n\nUSER QUERY: " + query + annotation + "\n\nRespond with valid JSON only:"

	log.Printf("🔄 Spektr Translator: query=\"%s\" schema=\"%s\"", truncate(query, 80), sch.Name)

	// 3. Call Gemini
	response, err := g.callGemini(fullPrompt)
	if err != nil {
		return nil, fmt.Errorf("gemini API error: %w", err)
	}

	// 4. Parse response
	result, err := parseResponse(response)
	if err != nil {
		log.Printf("⚠️ Spektr Translator: Parse failed, trying fallback: %v", err)

		// Fallback: return generic interpretation
		interp := parseFallbackInterpretation(response)
		return &TranslateResult{
			QuerySpec: engine.QuerySpec{
				Intent:      "table",
				Aggregation: "list",
				Visualize:   "table",
				Title:       "Query Results",
				Confidence:  0.5,
			},
			Interpretation: *interp,
		}, nil
	}

	log.Printf("✅ Spektr Translator: intent=%s, visualize=%s, confidence=%.2f",
		result.QuerySpec.Intent, result.QuerySpec.Visualize, result.QuerySpec.Confidence)

	return result, nil
}

// ============================================================================
// DATA SUMMARY BUILDER
// ============================================================================

// BuildDataSummaryFromRecords creates a lightweight DataSummary from records.
// This is what the AI sees — column names + unique values. Never raw data.
func BuildDataSummaryFromRecords(records []engine.Record, sch schema.Config) *DataSummary {
	if len(records) == 0 {
		return &DataSummary{
			RecordCount: 0,
			Dimensions:  map[string][]string{},
		}
	}

	dims := make(map[string]map[string]bool)
	for _, d := range sch.Dimensions {
		dims[d.Key] = make(map[string]bool)
	}

	for _, r := range records {
		for key, val := range r.Dimensions {
			if _, ok := dims[key]; ok && val != "" {
				dims[key][val] = true
			}
		}
	}

	summary := &DataSummary{
		RecordCount: len(records),
		Dimensions:  make(map[string][]string),
	}

	for key, valSet := range dims {
		vals := make([]string, 0, len(valSet))
		for v := range valSet {
			vals = append(vals, v)
		}
		summary.Dimensions[key] = vals
	}

	return summary
}

// ============================================================================
// GEMINI API CALL
// ============================================================================

// geminiRequest is the Gemini API request body.
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiResponse is the Gemini API response body.
type geminiResponse struct {
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

// callGemini sends a prompt to the Gemini API and returns the text response.
func (g *GeminiTranslator) callGemini(prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s",
		g.config.Endpoint, g.config.Model, g.config.APIKey)

	reqBody := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := g.client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var geminiResp geminiResponse
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
