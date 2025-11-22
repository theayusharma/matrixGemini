package main

import (
	"github.com/pelletier/go-toml/v2"
	"os"
)

type Config struct {
	Matrix  MatrixConfig  `toml:"matrix"`
	Gemini  GeminiConfig  `toml:"gemini"`
	Bot     BotConfig     `toml:"bot"`
	Credits CreditsConfig `toml:"credits"`
}

type MatrixConfig struct {
	Homeserver        string `toml:"homeserver"`
	UserID            string `toml:"user_id"`
	DeviceID          string `toml:"device_id"` // todo
	CredentialsDBPath string `toml:"credentials_db_path"`
	CryptoDBPath      string `toml:"crypto_db_path"`
	PickleKey         string `toml:"pickle_key"`
	AutoJoinInvites   bool   `toml:"auto_join_invites"`
}

type GeminiConfig struct {
	APIKey  string `toml:"api_key"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"`
}

type BotConfig struct {
	Name                   string  `toml:"name"`
	SystemPrompt           string  `toml:"system_prompt"`
	MaxResponseTokens      int     `toml:"max_response_tokens"`
	Temperature            float32 `toml:"temperature"`
	MaxConversationHistory int     `toml:"max_conversation_history"`
}

type CreditsConfig struct {
	FilePath    string `toml:"file_path"`
	GlobalLimit int    `toml:"global_limit"`
	MasterKey   string `toml:"master_key"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	err = toml.Unmarshal(data, &config)
	return &config, err
}
