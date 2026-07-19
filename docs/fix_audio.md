# Audio Quality Fixes

## Root Cause: Ogg Parser Bug

The noise (static/crackling) was caused by a bug in the Ogg page parser (`internal/music/stream.go`). The Ogg container format allows audio packets to span multiple 255-byte segments. The original parser treated every segment as a separate packet, splitting Opus frames in half and producing static.

### Fix: Continuation Segment Handling

```go
// Before (broken): each segment = separate packet
var packets [][]byte
for _, l := range segTable {
    if l == 0 { continue }
    packets = append(packets, data[offset:offset+int(l)])
    offset += int(l)
}

// After (fixed): accumulate 255-length segments into one packet
var packets [][]byte
var current []byte
for _, l := range segTable {
    if l == 0 { continue }
    current = append(current, data[offset:offset+int(l)]...)
    if l < 255 { // packet ends when segment < 255
        packets = append(packets, current)
        current = nil
    }
    offset += int(l)
}
```

## Fixes Applied

### 1. Ogg Packet Boundaries (static fix)

**File:** `internal/music/stream.go:137-150`

Segments with length `255` are continuation markers — the packet continues into the next segment. The parser now correctly accumulates these into a single packet.

### 2. Read/Send Decoupling (jitter fix)

**File:** `internal/music/stream.go:60-106`

Before: Ogg page reading and Opus sending were in the same goroutine, synchronized by a single ticker. If reading a page took longer than 20ms, the ticker drifted, causing audio gaps.

After: Reading runs in a separate goroutine that feeds a buffered channel (`pktCh`). The main goroutine reads from the channel at the ticker's pace (20ms), ensuring consistent frame timing.

### 3. Voice Connection Dead Detection (panic fix)

**File:** `internal/music/stream.go:83-103`

When the bot is force-disconnected from a voice channel, discordgo closes the `OpusSend` channel. Sending on a closed channel panics. All `select` blocks now include `case <-vc.Dead` to detect the disconnect and exit cleanly.

### 4. Force Disconnect Cleanup

**File:** `internal/bot/bot.go:66-78`

`onVoiceStateUpdate` now calls `ChannelVoiceJoin("")` to properly clean up the voice connection from the session, with a `disconnecting` guard to prevent infinite recursion.

**File:** `internal/music/player.go:74`

`Stop()` now clears `p.vc = nil` to prevent stale VoiceConnection references.

### 5. StreamSession Double-Close Fix

**File:** `internal/music/stream.go:16,59,155-161`

`Stop()` used to close `ss.done` (signal) and wait on `<-ss.done` (completion). Since the stream goroutine had `defer close(ss.done)`, the channel was closed twice → panic. Added a separate `finished` channel: `Stop()` closes `done` (signal) and waits on `finished` (completion). The stream goroutine closes `finished` when it exits.

## Audio Quality Improvements

| Setting | Before | After | Effect |
|---|---|---|---|
| Bitrate | 128k | 192k | Better Opus quality for music |
| Sample rate | source default | 48000 | Explicit 48kHz stereo |
| Channels | source default | 2 | Explicit stereo |
| Compression | default (5) | 10 | Max encoding quality |
| Video skip | none | `-vn` | Prevents video stream interference |
| Source format | `bestaudio` | `bestaudio[ext=m4a]/bestaudio` | Prefers AAC source for re-encoding |

## Pipeline

```
yt-dlp (extract audio URL)
  → ffmpeg (-vn -ar 48000 -ac 2 -c:a libopus -b:a 192k -compression_level 10 -frame_duration 20 -f ogg)
  → Go Ogg parser (continuation-aware)
  → 20ms ticker (decoupled from reading)
  → Discord Voice UDP
```

## Testing

```bash
# Test yt-dlp search
yt-dlp --default-search "ytsearch" --no-playlist --flat-playlist --print "%(title)s|||%(id)s|||%(duration)s" "ytsearch1:test song"

# Test ffmpeg Ogg output
ffmpeg -i "$AUDIO_URL" -c:a libopus -b:a 192k -f ogg pipe:1 | head -c 1000 | xxd
```