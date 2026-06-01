package llm

import (
	"fmt"
	"log"

	"github.com/Shardz4/raven/config"
)

// BuildProviders creates all available LLM providers from config.
// Returns the list of solver providers and a separate judge provider.
func BuildProviders(cfg *config.Config) (solvers []Provider, judge Provider, err error) {
	if cfg.OpenAIKey != "" {
		solvers = append(solvers, NewOpenAI(cfg.OpenAIKey, "gpt-4o"))
		solvers = append(solvers, NewOpenAI(cfg.OpenAIKey, "gpt-4o-mini"))
		log.Println("[llm] ✓ OpenAI providers registered (gpt-4o, gpt-4o-mini)")
	}
	if cfg.AnthropicKey != "" {
		solvers = append(solvers, NewAnthropic(cfg.AnthropicKey, "claude-sonnet-4-20250514"))
		log.Println("[llm] ✓ Anthropic provider registered (claude-sonnet-4-20250514)")
	}
	if cfg.DeepSeekKey != "" {
		solvers = append(solvers, NewDeepSeek(cfg.DeepSeekKey, "deepseek-chat"))
		log.Println("[llm] ✓ DeepSeek provider registered")
	}
	if cfg.GrokKey != "" {
		solvers = append(solvers, NewGrok(cfg.GrokKey, "grok-beta"))
		log.Println("[llm] ✓ Grok provider registered")
	}

	// Ollama — only include if reachable
	ollamaP := NewOllama(cfg.OllamaURL, "llama2")
	if ollamaP.IsAvailable() {
		solvers = append(solvers, ollamaP)
		log.Println("[llm] ✓ Ollama provider registered")
	} else {
		log.Println("[llm] ⚠ Ollama not reachable, skipping")
	}

	if len(solvers) == 0 {
		return nil, nil, fmt.Errorf("no LLM providers available — set at least one API key in .env")
	}

	// Build the judge provider (intentionally separate from solvers)
	judge, err = buildJudge(cfg, solvers)
	if err != nil {
		// Fallback: use the first solver as judge
		log.Printf("[llm] ⚠ Could not build dedicated judge (%v), falling back to first solver", err)
		judge = solvers[0]
	}

	return solvers, judge, nil
}

func buildJudge(cfg *config.Config, solvers []Provider) (Provider, error) {
	// judgeMaxTokens is lower than solver max tokens because the judge
	// only needs to return a small JSON array of scores, not full code.
	const judgeMaxTokens = 512

	switch cfg.JudgeProvider {
	case "openai":
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY required for openai judge")
		}
		p := NewOpenAI(cfg.OpenAIKey, cfg.JudgeModel)
		p.MaxTokens = judgeMaxTokens
		return p, nil
	case "anthropic":
		if cfg.AnthropicKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY required for anthropic judge")
		}
		p := NewAnthropic(cfg.AnthropicKey, cfg.JudgeModel)
		p.MaxTokens = judgeMaxTokens
		return p, nil
	case "deepseek":
		if cfg.DeepSeekKey == "" {
			return nil, fmt.Errorf("DEEPSEEK_API_KEY required for deepseek judge")
		}
		p := NewDeepSeek(cfg.DeepSeekKey, cfg.JudgeModel)
		p.inner.MaxTokens = judgeMaxTokens
		return p, nil
	case "grok":
		if cfg.GrokKey == "" {
			return nil, fmt.Errorf("XAI_API_KEY required for grok judge")
		}
		p := NewGrok(cfg.GrokKey, cfg.JudgeModel)
		p.inner.MaxTokens = judgeMaxTokens
		return p, nil
	case "custom":
		// Custom endpoint — plug in your own fine-tuned model
		if cfg.CustomJudgeURL == "" {
			return nil, fmt.Errorf("CUSTOM_JUDGE_URL required when JUDGE_PROVIDER=custom")
		}
		log.Printf("[llm] ✓ Custom judge registered: %s (%s)", cfg.CustomJudgeURL, cfg.CustomJudgeModel)
		return NewCustom(cfg.CustomJudgeURL, cfg.CustomJudgeModel, cfg.CustomJudgeKey), nil
	case "none", "skip":
		// No judge — consensus will use default neutral scores
		log.Println("[llm] ⚠ Judge phase disabled (JUDGE_PROVIDER=none)")
		return nil, nil
	default:
		// Try to use the first solver as fallback
		if len(solvers) > 0 {
			return solvers[0], nil
		}
		return nil, fmt.Errorf("unknown judge provider: %s", cfg.JudgeProvider)
	}
}

// BuildProvider builds a single LLM provider by name.
func BuildProvider(name string, cfg *config.Config) (Provider, error) {
	switch name {
	case "openai":
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("openai key not configured")
		}
		return NewOpenAI(cfg.OpenAIKey, "gpt-4o"), nil
	case "anthropic":
		if cfg.AnthropicKey == "" {
			return nil, fmt.Errorf("anthropic key not configured")
		}
		return NewAnthropic(cfg.AnthropicKey, "claude-sonnet-4-20250514"), nil
	case "deepseek":
		if cfg.DeepSeekKey == "" {
			return nil, fmt.Errorf("deepseek key not configured")
		}
		return NewDeepSeek(cfg.DeepSeekKey, "deepseek-chat"), nil
	case "grok":
		if cfg.GrokKey == "" {
			return nil, fmt.Errorf("grok key not configured")
		}
		return NewGrok(cfg.GrokKey, "grok-beta"), nil
	case "ollama":
		p := NewOllama(cfg.OllamaURL, "llama2")
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider %s", name)
	}
}
