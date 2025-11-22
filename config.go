package main

import (
	"github.com/pelletier/go-toml/v2"
	"os"

	"rakka/core"
	"rakka/core/llm"
	"rakka/platforms/discord"
	"rakka/platforms/matrix"
)

type Config struct {
	Matrix  matrix.Config      `toml:"matrix"`
	Discord discord.Config     `toml:"discord"`
	LLM     llm.Config         `toml:"llm"`
	Bot     core.BotConfig     `toml:"bot"`
	Credits core.CreditsConfig `toml:"credits"`
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
