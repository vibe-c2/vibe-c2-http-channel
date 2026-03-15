package config

import (
	"fmt"
	"os"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	"github.com/joho/godotenv"
)

type Config struct {
	ChannelID     string
	Listen        string
	C2SyncBaseURL string
	ProfilesFile  string
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// LoadFromEnv reads runtime configuration from environment variables.
// If envFilePath exists, it is loaded as a fallback source (.env style).
// Existing process environment values are not overridden by the .env file.
func LoadFromEnv(envFilePath string) (Config, error) {
	if envFilePath != "" {
		_ = godotenv.Load(envFilePath)
	}

	cfg := Config{
		ChannelID:     envOrDefault("CHANNEL_ID", "http-main"),
		Listen:        envOrDefault("LISTEN_ADDR", ":8080"),
		C2SyncBaseURL: envOrDefault("C2_SYNC_BASE_URL", "http://localhost:9000"),
		ProfilesFile:  envOrDefault("PROFILES_FILE", "examples/profiles/body-default.yaml"),
	}

	if cfg.C2SyncBaseURL == "" {
		return Config{}, fmt.Errorf("C2_SYNC_BASE_URL is required")
	}
	if cfg.ProfilesFile == "" {
		return Config{}, fmt.Errorf("PROFILES_FILE is required")
	}

	return cfg, nil
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
