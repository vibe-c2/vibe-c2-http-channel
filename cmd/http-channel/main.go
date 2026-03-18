package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/amqprpc"
	"github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/mgmtrpc"
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

	if cfg.AMQPURL != "" {
		conn, err := amqp.Dial(cfg.AMQPURL)
		if err != nil {
			log.Fatalf("amqp connect failed: %v", err)
		}
		defer conn.Close()

		rpcServer := mgmtrpc.NewServer(state)
		go func() {
			log.Printf("amqp rpc consumer started (queue=%s)", amqprpc.QueueName(cfg.ChannelID))
			if err := amqprpc.StartRPCConsumer(ctx, conn, cfg.ChannelID, rpcServer); err != nil && ctx.Err() == nil {
				log.Fatalf("amqp rpc consumer failed: %v", err)
			}
		}()
	}

	srv := httpserver.NewWithProvider(cfg.Listen, cfg.ChannelID, cfg.C2SyncBaseURL, state)
	log.Printf("vibe-c2-http-channel listening on %s (channel_id=%s, c2=%s, profiles=%d, profiles_dir=%s)", srv.Addr, cfg.ChannelID, cfg.C2SyncBaseURL, len(profiles), cfg.ProfilesDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
