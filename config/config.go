package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// TomlAuthor represents author configuration from TOML
type TomlAuthor struct {
	DID    string  `toml:"did"`
	Weight float64 `toml:"weight,omitempty,default=1.0"`
}

// TomlKeywords holds keyword configurations
type TomlKeywords map[string][]string

// TomlFilter represents a filter configuration
type TomlFilter struct {
	Type      string   `toml:"type"`
	Languages []string `toml:"languages,omitempty"`
	Include   []string `toml:"include,omitempty"` // References to keyword lists
	Exclude   []string `toml:"exclude,omitempty"` // References to keyword lists
}

// TomlScoring represents a scoring strategy configuration
type TomlScoring struct {
	Type     string       `toml:"type"`
	Weight   float64      `toml:"weight"`
	Keywords string       `toml:"keywords,omitempty"` // Reference to keyword list
	Authors  []TomlAuthor `toml:"authors,omitempty"`
}

// TomlFeed represents feed configuration
type TomlFeed struct {
	Id          string        `toml:"id"`
	DisplayName string        `toml:"display_name"`
	Description string        `toml:"description"`
	AvatarPath  string        `toml:"avatar_path"`
	Filters     []TomlFilter  `toml:"filters"`
	Scoring     []TomlScoring `toml:"scoring"`
}

// TomlConfig represents the top-level configuration
type TomlConfig struct {
	Keywords TomlKeywords `toml:"keywords"`
	Feeds    []TomlFeed   `toml:"feeds"`
}

func LoadConfig(path string) (*TomlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config TomlConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return &config, nil
}
