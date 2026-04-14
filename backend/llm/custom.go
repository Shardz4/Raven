package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CustomProvider connects to any HTTP endpoint that accepts a JSON prompt
// and returns a JSON response with a "score" or "content" field.
// This is the extension point for plugging in your own fine-tuned judge model.
//
// Expected API contract:
//   POST <base_url>
//   Body: {"prompt": "...", "patches": [...]}
//   Response: {"scores": [{"patch_index": 0, "score": 85}, ...]}
//
// OR the simpler OpenAI-compatible format:
//   POST <base_url>/v1/chat/completions
//   (standard OpenAI schema)
//
// Set JUDGE_PROVIDER=custom and CUSTOM_JUDGE_URL=<your-endpoint> in .env
type CustomProvider struct {
	baseURL string
	model   string
	apiKey  string
}

// NewCustom creates a custom judge provider pointing to your own endpoint.
func NewCustom(baseURL, model, apiKey string) *CustomProvider {
	return &CustomProvider{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
	}
}

func (c *CustomProvider) Name() string  { return "custom" }
func (c *CustomProvider) Model() string { return c.model }

func (c *CustomProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	// Try the simple Raven-native format first
	body := map[string]any{
		"prompt": prompt,
		"model":  c.model,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom judge request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("custom judge returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Try to parse as our native format: {"content": "...", "scores": [...]}
	var nativeResp struct {
		Content string `json:"content"`
		Scores  any    `json:"scores"`
	}
	if err := json.Unmarshal(respBody, &nativeResp); err == nil && nativeResp.Content != "" {
		return &PatchResult{
			Provider:    "custom",
			Model:       c.model,
			Code:        nativeResp.Content,
			Explanation: nativeResp.Content,
		}, nil
	}

	// Try OpenAI-compatible format
	var oaiResp openAIChatResponse
	if err := json.Unmarshal(respBody, &oaiResp); err == nil && len(oaiResp.Choices) > 0 {
		content := oaiResp.Choices[0].Message.Content
		return &PatchResult{
			Provider:    "custom",
			Model:       c.model,
			Code:        ExtractCode(content),
			Explanation: content,
		}, nil
	}

	// Last resort: treat the entire response as raw text
	return &PatchResult{
		Provider:    "custom",
		Model:       c.model,
		Code:        string(respBody),
		Explanation: string(respBody),
	}, nil
}
