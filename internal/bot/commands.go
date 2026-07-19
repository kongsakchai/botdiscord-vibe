package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/kong/botdiscord/internal/music"
	"github.com/kong/botdiscord/internal/queue"
)

func (b *Bot) registerCommandsForGuild(guildID string) {
	for _, cmd := range b.commandDefinitions() {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, guildID, cmd)
		if err != nil {
			log.Printf("Cannot create command %q in guild %s: %v", cmd.Name, guildID, err)
		}
	}
	log.Printf("Registered %d commands in guild %s", len(b.commandDefinitions()), guildID)
}

func (b *Bot) registerCommandsGlobally() {
	for _, cmd := range b.commandDefinitions() {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("Cannot create global command %q: %v", cmd.Name, err)
		}
	}
	log.Printf("Registered %d commands globally", len(b.commandDefinitions()))
}

func (b *Bot) commandDefinitions() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "play",
			Description: "Play a song from YouTube",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "YouTube URL or search query",
					Required:    true,
				},
			},
		},
		{
			Name:        "skip",
			Description: "Skip the current song",
		},
		{
			Name:        "stop",
			Description: "Stop playback and clear the queue",
		},
		{
			Name:        "pause",
			Description: "Pause playback",
		},
		{
			Name:        "resume",
			Description: "Resume playback",
		},
		{
			Name:        "queue",
			Description: "Show the current queue",
		},
		{
			Name:        "nowplaying",
			Description: "Show the currently playing song",
		},
		{
			Name:        "loop",
			Description: "Set loop mode",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "mode",
					Description: "Loop mode (off, one, queue)",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "off", Value: "off"},
						{Name: "one", Value: "one"},
						{Name: "queue", Value: "queue"},
					},
				},
			},
		},
		{
			Name:        "volume",
			Description: "Set playback volume",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "level",
					Description: "Volume level (0-100)",
					Required:    true,
					MinValue:    &[]float64{0}[0],
					MaxValue:    100,
				},
			},
		},
		{
			Name:        "remove",
			Description: "Remove a song from the queue",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "position",
					Description: "Position in queue (1-based)",
					Required:    true,
					MinValue:    &[]float64{1}[0],
				},
			},
		},
		{
			Name:        "clear",
			Description: "Clear the queue",
		},
	}
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()

	switch data.Name {
	case "play":
		b.handlePlay(s, i)
	case "skip":
		b.handleSkip(s, i)
	case "stop":
		b.handleStop(s, i)
	case "pause":
		b.handlePause(s, i)
	case "resume":
		b.handleResume(s, i)
	case "queue":
		b.handleQueue(s, i)
	case "nowplaying":
		b.handleNowPlaying(s, i)
	case "loop":
		b.handleLoop(s, i)
	case "volume":
		b.handleVolume(s, i)
	case "remove":
		b.handleRemove(s, i)
	case "clear":
		b.handleClear(s, i)
	}
}

func (b *Bot) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	query := i.ApplicationCommandData().Options[0].StringValue()

	var songs []*queue.Song

	if music.IsPlaylistURL(query) {
		results, err := music.GetPlaylistVideos(query)
		if err != nil || len(results) == 0 {
			editResponse(s, i, "Failed to get playlist or it's empty: "+err.Error())
			return
		}
		for _, r := range results {
			songs = append(songs, &queue.Song{
				Title:     r.Title,
				URL:       r.URL,
				Duration:  r.Duration,
				Requester: i.Member.User.Username,
			})
		}
	} else if music.IsYouTubeURL(query) {
		info, err := music.GetVideoInfo(query)
		if err != nil {
			editResponse(s, i, "Failed to get video info: "+err.Error())
			return
		}
		songs = append(songs, &queue.Song{
			Title:     info.Title,
			URL:       info.URL,
			Duration:  fmt.Sprintf("%.0f", info.Duration),
			Requester: i.Member.User.Username,
		})
	} else {
		results, err := music.Search(query, 1)
		if err != nil || len(results) == 0 {
			editResponse(s, i, "No results found for: "+query)
			return
		}
		r := results[0]
		songs = append(songs, &queue.Song{
			Title:     r.Title,
			URL:       r.URL,
			Duration:  r.Duration,
			Requester: i.Member.User.Username,
		})
	}

	if b.player.IsPlaying() {
		for _, song := range songs {
			b.player.GetQueue().Add(song)
		}
		msg := fmt.Sprintf("Added **%s** to queue", songs[0].Title)
		if len(songs) > 1 {
			msg = fmt.Sprintf("Added **%d** songs from playlist to queue", len(songs))
		}
		editResponse(s, i, msg)
		return
	}

	channelID := findUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		editResponse(s, i, "You must be in a voice channel.")
		return
	}

	vc, err := s.ChannelVoiceJoin(context.Background(), i.GuildID, channelID, false, true)
	if err != nil {
		editResponse(s, i, "Failed to join voice channel: "+err.Error())
		return
	}

	first := songs[0]
	rest := songs[1:]
	for _, song := range rest {
		b.player.GetQueue().Add(song)
	}

	if err := b.player.Play(first, vc); err != nil {
		editResponse(s, i, "Failed to play: "+err.Error())
		return
	}

	msg := fmt.Sprintf("Now playing: **%s**", first.Title)
	if len(rest) > 0 {
		msg += fmt.Sprintf(" (+ %d more from playlist)", len(rest))
	}
	editResponse(s, i, msg)
}

func (b *Bot) handleSkip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.player.IsPlaying() {
		respond(s, i, "Nothing is playing.")
		return
	}
	b.player.Skip()
	respond(s, i, "Skipped.")
}

func (b *Bot) handleStop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.player.Stop()
	s.ChannelVoiceJoin(context.Background(), i.GuildID, "", false, false)
	respond(s, i, "Stopped and cleared the queue.")
}

func (b *Bot) handlePause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.player.IsPlaying() {
		respond(s, i, "Nothing is playing.")
		return
	}
	if b.player.IsPaused() {
		respond(s, i, "Already paused.")
		return
	}
	b.player.Pause()
	respond(s, i, "Paused.")
}

func (b *Bot) handleResume(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.player.IsPaused() {
		respond(s, i, "Not paused.")
		return
	}
	if err := b.player.Resume(); err != nil {
		respond(s, i, "Failed to resume: "+err.Error())
		return
	}
	respond(s, i, "Resumed.")
}

func (b *Bot) handleQueue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	np := b.player.NowPlaying()
	list := b.player.GetQueue().List()

	var msg string
	if np != nil {
		msg += fmt.Sprintf("**Now Playing:** %s (requested by %s)\n\n", np.Title, np.Requester)
	}

	if len(list) == 0 {
		msg += "Queue is empty."
	} else {
		msg += fmt.Sprintf("**Queue (%d):**\n", len(list))
		for idx, song := range list {
			line := fmt.Sprintf("%d. %s (requested by %s)\n", idx+1, song.Title, song.Requester)
			if len(msg)+len(line) > 1950 {
				msg += fmt.Sprintf("... and %d more", len(list)-idx)
				break
			}
			msg += line
		}
	}

	respond(s, i, msg)
}

func (b *Bot) handleNowPlaying(s *discordgo.Session, i *discordgo.InteractionCreate) {
	np := b.player.NowPlaying()
	if np == nil {
		respond(s, i, "Nothing is playing.")
		return
	}

	loopStr := "off"
	switch b.player.GetLoopMode() {
	case music.LoopOne:
		loopStr = "one"
	case music.LoopQueue:
		loopStr = "queue"
	}

	msg := fmt.Sprintf("**Now Playing:** %s\nRequested by: %s\nLoop: %s\nVolume: %.0f%%",
		np.Title, np.Requester, loopStr, b.player.GetVolume()*100)

	respond(s, i, msg)
}

func (b *Bot) handleLoop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	mode := i.ApplicationCommandData().Options[0].StringValue()

	switch mode {
	case "off":
		b.player.SetLoopMode(music.LoopOff)
	case "one":
		b.player.SetLoopMode(music.LoopOne)
	case "queue":
		b.player.SetLoopMode(music.LoopQueue)
	}

	respond(s, i, "Loop mode set to: **"+mode+"**")
}

func (b *Bot) handleVolume(s *discordgo.Session, i *discordgo.InteractionCreate) {
	level := i.ApplicationCommandData().Options[0].IntValue()
	vol := float64(level) / 100.0
	b.player.SetVolume(vol)
	respond(s, i, fmt.Sprintf("Volume set to **%d%%**", level))
}

func (b *Bot) handleRemove(s *discordgo.Session, i *discordgo.InteractionCreate) {
	pos := i.ApplicationCommandData().Options[0].IntValue()
	song := b.player.GetQueue().Remove(int(pos) - 1)
	if song == nil {
		respond(s, i, "Invalid position.")
		return
	}
	respond(s, i, fmt.Sprintf("Removed: **%s**", song.Title))
}

func (b *Bot) handleClear(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.player.GetQueue().Clear()
	respond(s, i, "Queue cleared.")
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
}

func findUserVoiceChannel(s *discordgo.Session, guildID, userID string) string {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return ""
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return vs.ChannelID
		}
	}
	return ""
}
