package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Token    string
	AppID    string
	GuildIDs []string
}

func Load() *Config {
	godotenv.Load()

	raw := os.Getenv("DISCORD_GUILD_IDS")
	var guildIDs []string
	if raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				guildIDs = append(guildIDs, id)
			}
		}
	}

	return &Config{
		Token:    os.Getenv("DISCORD_TOKEN"),
		AppID:    os.Getenv("DISCORD_APP_ID"),
		GuildIDs: guildIDs,
	}
}