package llm

// DeepSeekProvider uses the OpenAI-compatible DeepSeek API.
type DeepSeekProvider struct {
	inner *OpenAIProvider
}

// NewDeepSeek creates a DeepSeek provider.
func NewDeepSeek(apiKey, model string) *DeepSeekProvider {
	p := NewOpenAI(apiKey, model)
	p.baseURL = "https://api.deepseek.com"
	return &DeepSeekProvider{inner: p}
}

func (d *DeepSeekProvider) Name() string  { return "deepseek" }
func (d *DeepSeekProvider) Model() string { return d.inner.model }

func (d *DeepSeekProvider) GeneratePatch(prompt string) (*PatchResult, error) {
	result, err := d.inner.GeneratePatch(prompt)
	if err != nil {
		return nil, err
	}
	result.Provider = "deepseek"
	result.Cost = 0 // DeepSeek pricing is minimal
	return result, nil
}
