package main

import (
	"log"
	"os"

	"github.com/vibe-c2/vibe-c2-http-channel/internal/config"
	httpserver "github.com/vibe-c2/vibe-c2-http-channel/internal/transport/http/httpserver"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	configFile := getEnv("CONFIG_FILE", "configs/channel.example.yaml")
	cfg, err := config.Load(configFile)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	profiles, err := config.LoadProfiles(cfg.ProfilesFile)
	if err != nil {
		log.Fatalf("profiles load failed: %v", err)
	}

	srv := httpserver.New(cfg.Listen, cfg.ChannelID, cfg.C2SyncBaseURL, profiles)
	log.Printf("vibe-c2-http-channel listening on %s (channel_id=%s, c2=%s, profiles=%d)", srv.Addr, cfg.ChannelID, cfg.C2SyncBaseURL, len(profiles))
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
