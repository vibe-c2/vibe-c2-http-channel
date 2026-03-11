package config

import (
	"fmt"
	"os"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ChannelID     string `yaml:"channel_id"`
	Listen        string `yaml:"listen"`
	C2SyncBaseURL string `yaml:"c2_sync_base_url"`
	ProfilesFile  string `yaml:"profiles_file"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}
	if c.ChannelID == "" {
		c.ChannelID = "http-main"
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.C2SyncBaseURL == "" {
		c.C2SyncBaseURL = "http://localhost:9000"
	}
	if c.ProfilesFile == "" {
		c.ProfilesFile = "configs/profiles.example.yaml"
	}
	return c, nil
}

func LoadProfiles(path string) ([]coreProfile.Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	profiles, err := coreProfile.ParseYAMLProfiles(b)
	if err != nil {
		return nil, fmt.Errorf("parse profiles yaml: %w", err)
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles loaded")
	}
	return profiles, nil
}
