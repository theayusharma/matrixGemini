package llm

import (
	"fmt"
	"strings"
)

type Config struct {
	Provider string `toml:"provider"`
	APIKey   string `toml:"api_key"`
	BaseURL  string `toml:"base_url"`
	Model    string `toml:"model"`
}

func New(cfg Config) (Provider, error) {
	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case "gemini":
			cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
		case "openai":
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	}

	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")

	switch cfg.Provider {
	case "gemini":
		return &GeminiProvider{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}, nil
	case "openai", "deepseek", "ollama":
		return &OpenAIProvider{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}, nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
}
