package main

import (
	"log"

	httpserver "github.com/vibe-c2/vibe-c2-http-channel/internal/transport/http/httpserver"
)

func main() {
	srv := httpserver.New(":8080")
	log.Printf("vibe-c2-http-channel listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
