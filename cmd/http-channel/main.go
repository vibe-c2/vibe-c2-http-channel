package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	state := config.NewProfilesState(profiles)
	if err := config.StartProfilesWatcher(ctx, cfg.ProfilesDir, state, log.Default()); err != nil {
		log.Fatalf("profiles watcher start failed: %v", err)
	}

	srv := httpserver.NewWithProvider(cfg.Listen, cfg.ChannelID, cfg.C2SyncBaseURL, state)
	log.Printf("vibe-c2-http-channel listening on %s (channel_id=%s, c2=%s, profiles=%d, profiles_dir=%s)", srv.Addr, cfg.ChannelID, cfg.C2SyncBaseURL, len(profiles), cfg.ProfilesDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
