package llm

import "os"

// GrokProvider uses the OpenAI-compatible xAI API.
type GrokProvider struct {
	inner *OpenAIProvider
}

// NewGrok creates a Grok/xAI provider.
func NewGrok(apiKey, model string) *GrokProvider {
	p := NewOpenAI(apiKey, model)
	baseURL := os.Getenv("XAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.x.ai"
	}
	p.baseURL = baseURL
	return &GrokProvider{inner: p}
}

func (g *GrokProvider) Name() string  { return "grok" }
func (g *GrokProvider) Model() string { return g.inner.model }

func (g *GrokProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	result, err := g.inner.GeneratePatch(prompt)
	if err != nil {
		return nil, err
	}
	result.Provider = "grok"
	return result, nil
}
