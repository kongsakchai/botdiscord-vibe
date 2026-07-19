## Setup

```bash
cp .env.example .env   # then fill DISCORD_TOKEN, DISCORD_APP_ID, DISCORD_GUILD_IDS
```

## Dependencies

- Go 1.22+
- `yt-dlp` (external binary, must be on PATH) — extracts YouTube audio URLs
- `ffmpeg` (external binary, must be on PATH) — converts audio to Opus for Discord

## Build & Run

Native:
```bash
go build -o botdiscord ./cmd/bot
./botdiscord
```

Docker (recommended — includes all deps):
```bash
cp .env.example .env   # fill DISCORD_TOKEN, DISCORD_APP_ID, DISCORD_GUILD_IDS
docker compose up --build -d
```

Single file test:
```bash
go test ./internal/... -run TestName
```

## Architecture

Entrypoint: `cmd/bot/main.go`

| Package | Role |
|---|---|
| `internal/config` | Env loading (DISCORD_TOKEN, etc.) |
| `internal/bot` | Discord session, slash command registration, voice join/leave |
| `internal/music` | Playback engine, yt-dlp integration, FFmpeg → Opus streaming |
| `internal/queue` | Thread-safe song queue |

## Audio Pipeline

```
yt-dlp (extract audio URL) → ffmpeg (libopus encoder, Ogg output) → Go Ogg parser → Discord Voice UDP (20ms frames)
```

## Key Gotchas

- **yt-dlp and ffmpeg must be installed on the host** — they are not Go libraries. The bot shells out to them.
- The Discord voice UDP connection requires sending Opus frames at exactly 20ms intervals. A ticker-based goroutine is the standard approach.
- Discord bot tokens and app IDs must be registered via the Discord Developer Portal with the `applications.commands` and `voice` scopes.
- Slash commands are registered globally by default; set `DISCORD_GUILD_IDS` (comma-separated, e.g. `123,456`) for instant guild-specific registration during development.
- **Volume changes take effect on the next song**, not mid-playback, because ffmpeg's volume filter is baked into the stream command.
- **`go.mod` uses `replace` directive** pointing to `yeongaori/discordgo-fork` — the upstream `bwmarrin/discordgo` does not support the DAVE/E2EE voice protocol (close code 4017). The fork changes `ChannelVoiceJoin` signature to require `context.Context` as first param.
- **`vc.Dead` channel** — the fork exposes `Dead <-chan struct{}` on `VoiceConnection` for detecting when voice is closed. All `select` blocks in `stream.go` listen on this to avoid panics on closed `OpusSend`.
- **`DISCORD_APP_ID` is loaded but never used** — `ApplicationCommandCreate` uses `session.State.User.ID` instead.
- **No CGo** — pure Go build. The original `gopkg.in/hraban/opus.v2` (CGo) was replaced with a custom Ogg page parser. No `CGO_ENABLED=1` needed.
- **Playlist limit** — `GetPlaylistVideos` caps at 10 songs via `--playlist-end 10` in yt-dlp.
- **No tests exist** — the repo has zero test files. `go test` returns nothing.
- **Dockerfile downloads yt-dlp from GitHub releases** — not from apk (Alpine package is often outdated).
- **`ss.finished` vs `ss.done`** — the `player.go` goroutine waits on `<-ss.finished`, not `<-ss.done`. `ss.done` signals stop, `ss.finished` signals stream exit. This matters for natural song-end detection.
- **Ogg parser** — `readOggPage` handles continuation segments (length 255 = packet continues). Without this, audio noise/static occurs. See `docs/fix_audio.md`.