package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey      string
	model       string
	temperature float64
}

// NewAnthropic creates a provider for the Anthropic Claude API.
func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:      apiKey,
		model:       model,
		temperature: 0.3,
	}
}

func (a *AnthropicProvider) Name() string  { return "anthropic" }
func (a *AnthropicProvider) Model() string { return a.model }

func (a *AnthropicProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	body := map[string]any{
		"model":      a.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": systemPrompt + "\n\n" + prompt},
		},
	}

	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	content := ""
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}
	code := ExtractCode(content)

	tokens := 0
	cost := 0.0
	if result.Usage != nil {
		tokens = result.Usage.InputTokens + result.Usage.OutputTokens
		cost = estimateAnthropicCost(a.model, result.Usage)
	}

	return &PatchResult{
		Provider:    "anthropic",
		Model:       a.model,
		Code:        code,
		Explanation: content,
		Tokens:      tokens,
		Cost:        cost,
	}, nil
}

func estimateAnthropicCost(model string, usage *anthropicUsage) float64 {
	if usage == nil {
		return 0
	}
	// Per 1M tokens pricing (input/output)
	switch {
	case containsStr(model, "opus"):
		return float64(usage.InputTokens)*15.0/1e6 + float64(usage.OutputTokens)*75.0/1e6
	case containsStr(model, "sonnet"):
		return float64(usage.InputTokens)*3.0/1e6 + float64(usage.OutputTokens)*15.0/1e6
	default: // haiku
		return float64(usage.InputTokens)*0.25/1e6 + float64(usage.OutputTokens)*1.25/1e6
	}
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
