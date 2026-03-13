// Package adapters provides reference AIProvider implementations.
// Consumers can use these directly or use them as a starting point
// for their own provider adapter.
//
// To implement your own adapter, satisfy the translator.AIProvider interface:
//
//	type AIProvider interface {
//	    Complete(prompt string) (string, error)
//	}
//
// Your adapter receives the fully-formed prompt string from Spektr and
// must return the raw text response from your AI provider. All HTTP
// transport, authentication, and request/response shaping is yours to own.
package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ============================================================================
// GEMINI ADAPTER — Reference implementation for Google Gemini
// ============================================================================
// Satisfies translator.AIProvider.
// Consumers targeting a different provider copy this file and adapt the
// request/response structs to match their provider's HTTP API shape.
// ============================================================================

// GeminiAdapter calls the Google Gemini API.
// Implements translator.AIProvider.
type GeminiAdapter struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewGeminiAdapter creates a GeminiAdapter.
//
//	adapter := adapters.NewGeminiAdapter("AIza...", "gemini-2.5-flash-lite")
//	t := translator.NewTranslator(adapter)
func NewGeminiAdapter(apiKey, model string) *GeminiAdapter {
	return &GeminiAdapter{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://generativelanguage.googleapis.com/v1beta/models",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// WithEndpoint overrides the default Gemini endpoint.
// Useful for Vertex AI or proxied endpoints.
func (a *GeminiAdapter) WithEndpoint(endpoint string) *GeminiAdapter {
	a.endpoint = endpoint
	return a
}

// Complete sends the prompt to Gemini and returns the text response.
// Implements translator.AIProvider.
func (a *GeminiAdapter) Complete(prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", a.endpoint, a.model, a.apiKey)

	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := a.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("Gemini error %d: %s", parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned empty response")
	}

	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

// ── Gemini-specific HTTP shapes (internal to this adapter) ────────────────

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}
type geminiPart struct {
	Text string `json:"text"`
}
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}