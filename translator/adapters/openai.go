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
// OPENAI ADAPTER — Reference implementation for OpenAI
// ============================================================================
// Satisfies translator.AIProvider.
//
// Key differences from the Gemini adapter:
//   - Auth via Authorization header (not query param)
//   - Request shape: messages array with role/content
//   - Response shape: choices[0].message.content
// ============================================================================

// OpenAIAdapter calls the OpenAI Chat Completions API.
// Implements translator.AIProvider.
//
// Also compatible with any OpenAI-compatible endpoint:
//   - Azure OpenAI (set endpoint to your Azure deployment URL)
//   - OpenRouter (set endpoint to https://openrouter.ai/api/v1)
//   - Local models via LM Studio or Ollama (set endpoint to localhost)
type OpenAIAdapter struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAIAdapter creates an OpenAIAdapter.
//
//	adapter := adapters.NewOpenAIAdapter("sk-...", "gpt-4o")
//	t := translator.NewTranslator(adapter)
func NewOpenAIAdapter(apiKey, model string) *OpenAIAdapter {
	return &OpenAIAdapter{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://api.openai.com/v1",
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// WithEndpoint overrides the default OpenAI endpoint.
// Use this for Azure OpenAI, OpenRouter, or any OpenAI-compatible API.
//
//	// Azure OpenAI
//	adapter.WithEndpoint("https://my-deployment.openai.azure.com/openai/deployments/my-model")
//
//	// OpenRouter
//	adapter.WithEndpoint("https://openrouter.ai/api/v1")
//
//	// Local (Ollama / LM Studio)
//	adapter.WithEndpoint("http://localhost:11434/v1")
func (a *OpenAIAdapter) WithEndpoint(endpoint string) *OpenAIAdapter {
	a.endpoint = endpoint
	return a
}

// Complete sends the prompt to the OpenAI Chat Completions API
// and returns the text response.
// Implements translator.AIProvider.
func (a *OpenAIAdapter) Complete(prompt string) (string, error) {
	url := fmt.Sprintf("%s/chat/completions", a.endpoint)

	body, err := json.Marshal(openAIRequest{
		Model: a.model,
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned empty response")
	}

	return parsed.Choices[0].Message.Content, nil
}

// ── OpenAI-specific HTTP shapes (internal to this adapter) ────────────────

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}