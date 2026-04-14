package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey      string
	model       string
	baseURL     string
	temperature float64
}

// NewOpenAI creates a provider for the OpenAI API.
func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:      apiKey,
		model:       model,
		baseURL:     "https://api.openai.com",
		temperature: 0.3,
	}
}

func (o *OpenAIProvider) Name() string  { return "openai" }
func (o *OpenAIProvider) Model() string { return o.model }

func (o *OpenAIProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	body := map[string]any{
		"model":       o.model,
		"temperature": o.temperature,
		"max_tokens":  4096,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
	}
	return o.call(body)
}

func (o *OpenAIProvider) call(body map[string]any) (*PatchResult, error) {
	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", o.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai returned 0 choices")
	}

	content := result.Choices[0].Message.Content
	code := ExtractCode(content)

	tokens := 0
	if result.Usage != nil {
		tokens = result.Usage.TotalTokens
	}

	return &PatchResult{
		Provider:    "openai",
		Model:       o.model,
		Code:        code,
		Explanation: content,
		Tokens:      tokens,
		Cost:        estimateOpenAICost(o.model, result.Usage),
	}, nil
}

func estimateOpenAICost(model string, usage *openAIUsage) float64 {
	if usage == nil {
		return 0
	}
	// Approximate pricing per 1M tokens (input/output)
	switch {
	case contains(model, "gpt-4o-mini"):
		return float64(usage.PromptTokens)*0.15/1e6 + float64(usage.CompletionTokens)*0.6/1e6
	case contains(model, "gpt-4o"):
		return float64(usage.PromptTokens)*2.5/1e6 + float64(usage.CompletionTokens)*10.0/1e6
	case contains(model, "gpt-4-turbo"):
		return float64(usage.PromptTokens)*10.0/1e6 + float64(usage.CompletionTokens)*30.0/1e6
	default:
		return float64(usage.PromptTokens)*0.5/1e6 + float64(usage.CompletionTokens)*1.5/1e6
	}
}

// --- shared types for OpenAI-compatible APIs ---

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

const systemPrompt = `You are an expert programmer. Generate a high-quality code fix that:
- Is production-ready and correct
- Includes proper error handling
- Has clear comments explaining the fix
- Follows language best practices

Return ONLY valid code in a markdown code block. No commentary outside the block.`
