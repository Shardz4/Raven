package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaProvider implements Provider for local Ollama models.
type OllamaProvider struct {
	baseURL string
	model   string
}

// NewOllama creates a provider for a local Ollama instance.
func NewOllama(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
	}
}

func (o *OllamaProvider) Name() string  { return "ollama" }
func (o *OllamaProvider) Model() string { return o.model }

// IsAvailable checks if the Ollama server is reachable.
func (o *OllamaProvider) IsAvailable() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(o.baseURL + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (o *OllamaProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	body := map[string]any{
		"model":  o.model,
		"prompt": systemPrompt + "\n\n" + prompt,
		"stream": false,
	}
	payload, _ := json.Marshal(body)

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Post(o.baseURL+"/api/generate", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response  string `json:"response"`
		EvalCount int    `json:"eval_count"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	code := ExtractCode(result.Response)

	return &PatchResult{
		Provider:    "ollama",
		Model:       o.model,
		Code:        code,
		Explanation: result.Response,
		Tokens:      result.EvalCount,
		Cost:        0, // Self-hosted
	}, nil
}
