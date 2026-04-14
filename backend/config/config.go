package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	Port string

	// GitHub
	GitHubToken string

	// LLM API Keys
	OpenAIKey    string
	AnthropicKey string
	DeepSeekKey  string
	GrokKey      string
	OllamaURL    string

	// Consensus
	JudgeProvider string // Which LLM provider to use as the judge (e.g., "openai", "anthropic")
	JudgeModel    string // Which model to use as judge (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Redundancy    int    // Number of LLMs to fan-out to

	// Sandbox
	DockerTimeout int    // Seconds
	SandboxImage  string // Docker image tag for sandbox

	// Database
	DBPath string

	// Bots
	TelegramToken string
	DiscordToken  string
}

// Load reads .env from the project root (one level up from backend/) and populates Config.
func Load() *Config {
	// Try loading .env from project root
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	cfg := &Config{
		Port:          envOrDefault("PORT", "8080"),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		OpenAIKey:     os.Getenv("OPENAI_API_KEY"),
		AnthropicKey:  os.Getenv("ANTHROPIC_API_KEY"),
		DeepSeekKey:   os.Getenv("DEEPSEEK_API_KEY"),
		GrokKey:       firstNonEmpty(os.Getenv("XAI_API_KEY"), os.Getenv("GROK_API_KEY")),
		OllamaURL:     envOrDefault("OLLAMA_URL", "http://localhost:11434"),
		JudgeProvider: envOrDefault("JUDGE_PROVIDER", "openai"),
		JudgeModel:    envOrDefault("JUDGE_MODEL", "gpt-4o"),
		Redundancy:    envOrDefaultInt("RAVEN_REDUNDANCY", 3),
		DockerTimeout: envOrDefaultInt("DOCKER_TIMEOUT", 60),
		SandboxImage:  envOrDefault("SANDBOX_IMAGE", "raven-sandbox:latest"),
		DBPath:        envOrDefault("DB_PATH", "raven.db"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		DiscordToken:  os.Getenv("DISCORD_BOT_TOKEN"),
	}

	log.Printf("[config] Loaded — port=%s, redundancy=%d, judge=%s/%s", cfg.Port, cfg.Redundancy, cfg.JudgeProvider, cfg.JudgeModel)
	return cfg
}

// AvailableProviders returns a list of provider names that have API keys configured.
func (c *Config) AvailableProviders() []string {
	var providers []string
	if c.OpenAIKey != "" {
		providers = append(providers, "openai")
	}
	if c.AnthropicKey != "" {
		providers = append(providers, "anthropic")
	}
	if c.DeepSeekKey != "" {
		providers = append(providers, "deepseek")
	}
	if c.GrokKey != "" {
		providers = append(providers, "grok")
	}
	// Ollama is always potentially available (local)
	providers = append(providers, "ollama")
	return providers
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
