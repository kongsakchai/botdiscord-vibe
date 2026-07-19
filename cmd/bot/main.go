package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kong/botdiscord/internal/bot"
	"github.com/kong/botdiscord/internal/config"
)

func main() {
	cfg := config.Load()

	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	log.Println("Bot is running. Press Ctrl+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}