package main

import (
	"log"
	"os"

	httpserver "github.com/vibe-c2/vibe-c2-http-channel/internal/transport/http/httpserver"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := getEnv("LISTEN_ADDR", ":8080")
	channelID := getEnv("CHANNEL_ID", "http-main")
	c2SyncBaseURL := getEnv("C2_SYNC_BASE_URL", "http://localhost:9000")

	srv := httpserver.New(addr, channelID, c2SyncBaseURL)
	log.Printf("vibe-c2-http-channel listening on %s (channel_id=%s, c2=%s)", srv.Addr, channelID, c2SyncBaseURL)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
