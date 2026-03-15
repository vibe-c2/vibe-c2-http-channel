package main

import (
	"flag"
	"log"

	"github.com/vibe-c2/vibe-c2-http-channel/internal/config"
	httpserver "github.com/vibe-c2/vibe-c2-http-channel/internal/transport/http/httpserver"
)

func main() {
	var envFile string
	flag.StringVar(&envFile, "config", ".env", "path to .env fallback file")
	flag.Parse()

	cfg, err := config.LoadFromEnv(envFile)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	if err := config.EnsureProfilesDir(cfg.ProfilesDir); err != nil {
		log.Fatalf("profiles dir init failed: %v", err)
	}
	profiles, err := config.LoadProfiles(cfg.ProfilesDir)
	if err != nil {
		log.Fatalf("profiles load failed: %v", err)
	}

	srv := httpserver.New(cfg.Listen, cfg.ChannelID, cfg.C2SyncBaseURL, profiles)
	log.Printf("vibe-c2-http-channel listening on %s (channel_id=%s, c2=%s, profiles=%d, profiles_dir=%s)", srv.Addr, cfg.ChannelID, cfg.C2SyncBaseURL, len(profiles), cfg.ProfilesDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
