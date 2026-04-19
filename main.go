package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "dev" // overridden by -ldflags at build time

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[reddit-monitor] ")

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config: %v\n\nCreate a .env file with:\n  DISCORD_WEBHOOK_URL=...\n", err)
	}

	// Under systemd, STATE_DIRECTORY is set to /var/lib/reddit-monitor.
	// Local dev falls back to the current directory.
	stateDir := os.Getenv("STATE_DIRECTORY")
	if stateDir == "" {
		stateDir = "."
	}
	seen, err := NewSeenStore(filepath.Join(stateDir, "seen.json"))
	if err != nil {
		log.Fatalf("seen store: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	RunMonitor(ctx, stop, seen, func(m MatchResult) error {
		return SendWebhook(cfg.DiscordWebhookURL, m)
	}, func(title, desc string) error {
		return SendInfoEmbed(cfg.DiscordWebhookURL, title, desc)
	}, 5*time.Minute)

	log.Println("Shutting down")
}
