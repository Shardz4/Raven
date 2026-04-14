package llm

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PatchResult is the output from a single LLM when asked to fix an issue.
type PatchResult struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Code        string  `json:"code"`
	Explanation string  `json:"explanation"`
	Tokens      int     `json:"tokens"`
	Cost        float64 `json:"cost"`
	DurationMs  int64   `json:"duration_ms"`
}

// Provider is the interface every LLM backend must implement.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string
	// Model returns the model name in use.
	Model() string
	// GeneratePatch sends the issue prompt to the LLM and returns a code patch.
	GeneratePatch(prompt string) (*PatchResult, error)
}

// FanOut calls all given providers concurrently and returns results as they arrive.
// The callback `onResult` is called safely from goroutines (synchronized via mutex).
func FanOut(providers []Provider, prompt string, onEvent func(event string)) []*PatchResult {
	var mu sync.Mutex
	var results []*PatchResult
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(prov Provider) {
			defer wg.Done()

			name := fmt.Sprintf("%s/%s", prov.Name(), prov.Model())

			mu.Lock()
			onEvent(fmt.Sprintf("⚡ Prompting %s...", name))
			mu.Unlock()

			start := time.Now()
			result, err := prov.GeneratePatch(prompt)
			if err != nil {
				mu.Lock()
				onEvent(fmt.Sprintf("❌ %s failed: %v", name, err))
				mu.Unlock()
				return
			}
			result.DurationMs = time.Since(start).Milliseconds()

			mu.Lock()
			results = append(results, result)
			onEvent(fmt.Sprintf("📦 Received patch from %s (%dms)", name, result.DurationMs))
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return results
}

// ExtractCode pulls Python code out of a markdown-formatted LLM response.
func ExtractCode(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Try ```python\n...\n```
	re := regexp.MustCompile("(?s)```(?:python)?\\n(.*?)\\n```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}

	// Try single backtick `...`
	reSingle := regexp.MustCompile("`([^`]+)`")
	if m := reSingle.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}

	return text
}
