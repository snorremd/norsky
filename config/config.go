package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type FeedConfig struct {
	ID              string   `toml:"id"`
	DisplayName     string   `toml:"display_name"`
	Description     string   `toml:"description"`
	AvatarPath      string   `toml:"avatar_path"`
	Languages       []string `toml:"languages"`
	Keywords        []string `toml:"keywords"`
	ExcludeKeywords []string `toml:"exclude_keywords,omitempty"`
	ExcludeReplies  bool     `toml:"exclude_replies,omitempty" default:"false"`
}

type Config struct {
	Feeds []FeedConfig `toml:"feeds"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return &config, nil
}
