# Design System — Discord YouTube Music Bot

## Tech Stack

| Layer | Technology | Choice Rationale |
|---|---|---|
| Language | Go 1.22+ | Good concurrency model, single binary deploy, large standard library |
| Discord API | `github.com/yeongaori/discordgo-fork` (fork of `bwmarrin/discordgo`) | Full DAVE/E2EE voice protocol support (not yet in upstream) |
| Audio source | `yt-dlp` (external binary) | Extracts YouTube audio URLs; the successor to `youtube-dl`, actively maintained |
| Audio encoding | `ffmpeg` (external binary) | Converts YouTube audio to Opus; libopus encoder, Ogg container output |
| Ogg parsing | Pure Go (custom) | Parses Ogg container to extract raw Opus frames; avoids CGo `libopus` dependency |
| Env loading | `github.com/joho/godotenv` | Standard `.env` file loading |
| Cryptography | `github.com/cloudflare/circl` | MLS (Message Layer Security) for DAVE/E2EE key exchange (indirect dep) |

## Architecture

### Project Structure

```
cmd/bot/main.go          — Entrypoint
internal/
  config/config.go       — Env loading (DISCORD_TOKEN, DISCORD_GUILD_IDS, etc.)
  bot/bot.go             — Discord session, event handlers, gateway management
  bot/commands.go        — Slash command definitions and handlers
  music/youtube.go       — yt-dlp integration (search, video info, audio URL, playlist)
  music/stream.go        — ffmpeg → Ogg → Opus streaming pipeline
  music/player.go        — Playback engine (queue, skip, loop, pause/resume, volume)
  queue/queue.go         — Thread-safe song queue
```

### Package Dependencies

```
cmd/bot/main.go
  → internal/config
  → internal/bot
    → internal/config
    → internal/music
      → internal/queue
      → github.com/bwmarrin/discordgo (fork)
      → yt-dlp (external binary)
      → ffmpeg (external binary)
    → internal/queue
```

### Entrypoint Flow

```
main.go
  ├── config.Load()          → reads .env, returns Config
  ├── bot.New(cfg)           → creates discordgo session, registers handlers, opens gateway
  │   ├── onReady            → registers slash commands (per-guild or global)
  │   ├── onGuildCreate      → logs when bot joins a new server
  │   ├── onInteractionCreate → dispatches /play, /skip, /stop, etc.
  │   └── onVoiceStateUpdate → handles force-disconnect cleanup
  ├── signal.Notify          → waits for SIGINT/SIGTERM
  └── bot.Close()            → stops player, closes session
```

## Audio Pipeline

### End-to-End Flow

```
1. User runs /play <query>
2. yt-dlp resolves query → YouTube video metadata (title, duration, uploader)
3. yt-dlp extracts best audio URL → direct media URL
4. ffmpeg downloads audio URL, applies volume filter, encodes to Opus, muxes to Ogg
5. Go Ogg parser reads ffmpeg stdout, extracts Opus frames from Ogg pages
6. 20ms ticker paces sending of Opus frames to Discord voice gateway
7. Discord voice UDP receives Opus frames, plays audio in voice channel
```

### ffmpeg Command

```
ffmpeg
  -reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 5
  -vn
  -i <audio_url>
  -af "volume=1.00"
  -c:a libopus -b:a 192k -application audio -compression_level 10
  -ar 48000 -ac 2 -frame_duration 20
  -f ogg -loglevel quiet
  pipe:1
```

Key parameters:
- `-b:a 192k` — Opus quality target (music-grade)
- `-application audio` — Opus mode optimized for music (vs VoIP)
- `-compression_level 10` — maximum encoding quality
- `-frame_duration 20` — 20ms frames (required by Discord voice protocol)
- `-ar 48000 -ac 2` — explicit 48kHz stereo resampling
- `-f ogg` — Ogg container output (raw Opus frames wrapped in Ogg pages)

### Ogg Parsing (Pure Go)

```
ffmpeg → Ogg stream → Go reads 27-byte page headers → segment table → data segments
  → BOS pages (OpusHead, OpusTags) SKIPPED
  → Audio pages: continuation segments (255-byte) ACCUMULATED into single packets
  → Each packet = one 20ms Opus frame → sent to Discord at 20ms intervals
```

The Ogg parser handles:
- Continuation markers (segments with length 255 = packet continues into next segment)
- BOS (Begin Of Stream) metadata pages (OpusHead, OpusTags) — skipped
- Multiple packets per page — extracted and sent individually

## Data Structures

### Song (queue.go)

```go
type Song struct {
    Title     string   // YouTube video title
    URL       string   // YouTube watch URL
    Duration  string   // Duration string (e.g. "214")
    Requester string   // Discord username who requested
}
```

### Player (player.go)

```go
type Player struct {
    q             *Queue             // Song queue (thread-safe)
    current       *Song              // Currently playing song
    session       *StreamSession     // Active ffmpeg stream
    vc            *VoiceConnection   // Discord voice connection
    loopMode      LoopMode           // off | one | queue
    paused        bool               // Whether playback is paused
    volume        float64            // Volume multiplier (0.0-1.0)
    skipRequested bool               // Flag for skip/stop to prevent loop
}
```

### StreamSession (stream.go)

```go
type StreamSession struct {
    cmd      *exec.Cmd     // ffmpeg process
    stdout   io.ReadCloser // ffmpeg stdout pipe
    done     chan struct{} // Signal to stop the stream
    finished chan struct{} // Signal that stream has exited
}
```

## State Machine

### Playback States

```
IDLE → PLAYING → SONG_END → PLAYING (next song)
                → SKIP    → PLAYING (next song)
                → STOP    → IDLE
                → PAUSE   → PAUSED → RESUME → PLAYING
                → FORCE_DISCONNECT → IDLE
```

### Player Mutex Locking

The `Player` uses a single `sync.Mutex` (`p.mu`). Lock ordering:

1. `Player.Play()`, `Skip()`, `Stop()`, `Pause()`, `Resume()` — acquire lock, call `startStreamLocked()` (assumes lock held)
2. `onSongEnd()` — acquires lock, calls `startStreamLocked()` (assumes lock held)
3. `startStreamLocked()` — assumes lock is already held
4. `GetQueue()`, `IsPlaying()`, `IsPaused()`, `NowPlaying()` — acquire lock, read state, release

## Slash Commands

| Command | Description | Options |
|---|---|---|
| `/play <query>` | Play from URL, search, or playlist | `query`: URL or search text |
| `/skip` | Skip to next song | — |
| `/stop` | Stop playback, clear queue, leave VC | — |
| `/pause` | Pause playback | — |
| `/resume` | Resume playback | — |
| `/queue` | Show current queue | — |
| `/nowplaying` | Show current song info | — |
| `/loop <mode>` | Set loop mode | `mode`: off, one, queue |
| `/volume <level>` | Set volume (0-100) | `level`: integer 0-100 |
| `/remove <position>` | Remove a song from queue | `position`: 1-based index |
| `/clear` | Clear the queue | — |

### Command Registration

- If `DISCORD_GUILD_IDS` is set: commands registered to each guild ID (instant propagation, ideal for development)
- If empty: commands registered globally (up to 1 hour propagation delay)

## External Dependencies

### System Binaries

| Binary | Version | Purpose | Installation |
|---|---|---|---|
| `ffmpeg` | latest | Audio encoding (libopus) | `apt install ffmpeg` or `apk add ffmpeg` |
| `yt-dlp` | latest | YouTube audio extraction | Download from GitHub releases |
| `libopus` (runtime) | latest | Opus codec for ffmpeg | `apt install libopus0` or `apk add libopus` |

### Go Dependencies

```
github.com/yeongaori/discordgo-fork (fork of bwmarrin/discordgo)
  - Discord API + voice gateway
  - DAVE/E2EE voice protocol support
  - Forked from commit 54ae40de + additional DAVE implementation

github.com/joho/godotenv — .env file loading
github.com/cloudflare/circl — MLS cryptography (indirect, via discordgo fork)
github.com/gorilla/websocket — WebSocket (indirect, via discordgo)
golang.org/x/crypto — crypto (indirect, via discordgo)
```

## Design Decisions

### Why Pure Go Ogg Parsing (no CGo)

- **Initial approach**: CGo `libopus` with `gopkg.in/hraban/opus.v2` — Go reads PCM from ffmpeg, encodes to Opus via CGo
- **Current approach**: Pure Go — ffmpeg encodes to Opus internally, outputs Ogg container, Go parses Ogg pages
- **Reason**: Eliminate CGo dependency, simplify build (`go build` without `CGO_ENABLED=1` or `libopus-dev`)
- **Tradeoff**: Volume changes take effect on next song (not mid-song), because ffmpeg's volume filter is baked into the command

### Why discordgo Fork

- Upstream `bwmarrin/discordgo` doesn't support the DAVE/E2EE protocol required by Discord's voice servers (close code 4017)
- `yeongaori/discordgo-fork` implements the full DAVE protocol: MLS key exchange, epoch transitions, AEAD encryption
- PR #1704 has been submitted upstream but not yet merged

### Why yt-dlp (not direct API)

- YouTube's API is frequently blocked/changed; yt-dlp adapts to these changes
- No API key needed
- Supports searching, playlist extraction, and format selection
- Tradeoff: external binary dependency, must be installed on the system

### Thread Safety

- `Player` uses `sync.Mutex` for all state modifications
- `Queue` has its own `sync.Mutex` for concurrent access from command handlers and playback goroutine
- `StreamSession` uses channels (`done`, `finished`) for signaling between goroutines
- Voice connection dead detection uses `vc.Dead` channel (from discordgo fork)

## Configuration

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DISCORD_TOKEN` | Yes | Discord bot token |
| `DISCORD_APP_ID` | No | Discord application ID |
| `DISCORD_GUILD_IDS` | No | Comma-separated guild IDs for instant command registration |

## Building

### Docker (Recommended)

```bash
cp .env.example .env
docker compose up --build -d
```

The Dockerfile uses a two-stage build:
1. **Builder**: `golang:1.22-alpine` — compiles Go binary
2. **Runtime**: `alpine:3.19` — includes `ffmpeg`, `libopus`, and `yt-dlp` (downloaded from GitHub releases)

### Native

```bash
go build -o botdiscord ./cmd/bot
./botdiscord
```

Requires `ffmpeg` and `yt-dlp` on PATH.

## Known Limitations

- Volume changes take effect on the next song (not mid-playback)
- Bot supports one voice channel per session (single Player instance)
- No `/search` interactive select menu (results returned as text only)
- Pause/resume restarts from the beginning of the song (not resuming position)
- Playlist flat extraction (`--flat-playlist`) returns only title, URL, and duration — no detailed per-video metadata