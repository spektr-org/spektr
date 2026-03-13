package adapters

import "github.com/spektr-org/spektr/translator"

// New returns the appropriate AIProvider adapter based on the endpoint.
// Consumers who want explicit control can instantiate NewGeminiAdapter
// or NewOpenAIAdapter directly. This factory is for the api package
// which receives endpoint/key/model as plain strings from the caller.
//
// Selection rules:
//   - endpoint contains "openai" or "openrouter" → OpenAIAdapter
//   - everything else (empty, Gemini, Vertex, custom) → GeminiAdapter
//
// This is the only place in Spektr where provider selection lives.
func New(apiKey, model, endpoint string) translator.AIProvider {
	if isOpenAICompatible(endpoint) {
		a := NewOpenAIAdapter(apiKey, model)
		if endpoint != "" {
			a.WithEndpoint(endpoint)
		}
		return a
	}

	a := NewGeminiAdapter(apiKey, model)
	if endpoint != "" {
		a.WithEndpoint(endpoint)
	}
	return a
}

func isOpenAICompatible(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	for _, marker := range []string{"openai", "openrouter", "azure"} {
		if contains(endpoint, marker) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}