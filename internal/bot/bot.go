package bot

import (
	"context"
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/kong/botdiscord/internal/config"
	"github.com/kong/botdiscord/internal/music"
)

type Bot struct {
	session      *discordgo.Session
	config       *config.Config
	player       *music.Player
	disconnecting bool
}

func New(cfg *config.Config) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, err
	}

	b := &Bot{
		session: s,
		config:  cfg,
		player:  music.NewPlayer(),
	}

	if err := music.InitSearchCache("search_cache.db"); err != nil {
		log.Printf("Warning: search cache not available: %v", err)
	}

	b.player.SetOnIdleTimeout(func(guildID string) {
		log.Printf("Idle timeout: disconnecting from guild %s", guildID)
		s.ChannelVoiceJoin(context.Background(), guildID, "", false, false)
	})

	s.AddHandler(b.onReady)
	s.AddHandler(b.onGuildCreate)
	s.AddHandler(b.onInteractionCreate)
	s.AddHandler(b.onVoiceStateUpdate)

	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates

	if err := s.Open(); err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bot) Close() {
	b.player.Stop()
	b.session.Close()
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Logged in as %s#%s", r.User.Username, r.User.Discriminator)

	if len(b.config.GuildIDs) > 0 {
		for _, gid := range b.config.GuildIDs {
			b.registerCommandsForGuild(gid)
		}
	} else {
		b.registerCommandsGlobally()
	}
}

func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	log.Printf("Joined guild: %s (ID: %s)", g.Name, g.ID)
}

func (b *Bot) onVoiceStateUpdate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
	if vs.UserID != s.State.User.ID {
		return
	}
	if vs.ChannelID == "" && !b.disconnecting {
		b.disconnecting = true
		defer func() { b.disconnecting = false }()

		log.Printf("Disconnected from voice in guild %s", vs.GuildID)
		b.player.Stop()
		s.ChannelVoiceJoin(context.Background(), vs.GuildID, "", false, false)
	}
}