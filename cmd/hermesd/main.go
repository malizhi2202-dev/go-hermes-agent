package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"hermes-agent/go/internal/api"
	"hermes-agent/go/internal/app"
	"hermes-agent/go/internal/config"
)

func main() {
	configPath := flag.String("config", "./configs/config.example.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	server := api.New(application)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.ListenAndServe(ctx); err != nil && err.Error() != "http: Server closed" {
		log.Fatalf("server error: %v", err)
	}
}
