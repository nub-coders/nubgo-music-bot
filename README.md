# NUB Go Music Bot

A Telegram group voice-chat music bot written in Go, using the NUB Gogram fork (`github.com/nub-coders/gogram`), NTgCalls, FFmpeg, yt-dlp, and optional MongoDB persistence.

## Current MVP

- Audio and 720p video playback from YouTube search/URLs
- Python-compatible resolver priority: InnerTube → optional NUB API → optional YouTube Data API → yt-dlp
- Direct HTTP(S) streams with public-network URL validation
- Per-chat serialized queues
- Hot track switching without leaving and rejoining the voice chat
- Pause, resume, skip, stop, queue, and loop controls
- Multiple assistant accounts with least-loaded allocation
- Auto-leave of idle chats so assistants stay under Telegram's 500-group limit
- Owner, sudo, Telegram-admin, and authorized-user permissions
- MongoDB compatibility with the existing Python bot's access and playback collections
- Graceful shutdown of playback, native calls, assistants, and storage

## Commands

| Command | Description |
| --- | --- |
| `/play` | Resolve and play a song, URL, or search query (video: `/vplay`) |
| `/playforce` / `/vplayforce` | Interrupt the current track and play immediately, keeping the queue |
| `/cplay` / `/cvplay` | Play into the linked channel/discussion chat |
| `/cplayforce` / `/cvplayforce` | Force-play into the linked chat |
| `/pause` / `/resume` | Toggle playback |
| `/skip` | Skip to the next track |
| `/stop` / `/end` | Stop playback and clear the queue |
| `/queue` / `/q` | Show the queue |
| `/loop off\|track\|queue` | Set loop mode |
| `/shuffle` | Shuffle the queue (Fisher–Yates) |
| `/seek N` / `/seek 1:30` / `/seekback N` | Seek forward / absolute / backward |
| `/np` / `/nowplaying` | Show the inline now-playing card |
| `/playlist new NAME` / `del NAME` / `add NAME URL` / `rm NAME INDEX` / `show NAME` | Manage per-user playlists |
| `/myplaylist` | List your playlists |
| `/pl NAME` / `/pplay NAME` | Play a saved playlist |
| `/auth` / `/unauth` / `/authlist` | Authorize users in a group |
| `/block` / `/unblock` / `/blocklist` | Block users from playback |
| `/sudo` / `/delsudo` / `/sudolist` | Owner-only sudo management |
| `/broadcast` / `/fbroadcast` | Broadcast a message to all chats |
| `/setwelcome` / `/welcome` | Per-chat welcome message |
| `/stats` | Top chats by play count |
| `/start` / `/help` / `/ping` / `/about` | General commands |

Inline buttons on the now-playing card provide pause/resume, skip, loop, stop, and queue actions.

Force-play is allowed for admins, sudo users, and authorized users, plus the user who requested the track currently playing. The replacement is resolved before the running track is interrupted, so an unresolvable query leaves playback untouched.

Channel commands (`/cplay` and friends) stream into the chat linked to the current one; the now-playing card and control buttons stay in the chat where the command was sent. `/cskip`, `/cstop`, and `/cend` are accepted as aliases.

Container links are expanded before enqueueing: Spotify track/album/playlist (client-credentials API) and YouTube playlists (yt-dlp flat extraction, capped at 50 items).

## Requirements

- Linux x86-64 (the bundled NTgCalls library is architecture-specific)
- Go 1.25+
- CGO and a C/C++ compiler for builds
- FFmpeg
- yt-dlp
- A bot token and at least one authorized assistant user session

Copy `.env.example` to `.env`, configure it, then run:

```bash
make test
make build
./bin/musicbot
```

The bot expects an active group voice chat when `/play` is used. MongoDB is optional for playback, but required for persistent `/auth` and sudo changes.

The NUB API and YouTube Data API are optional. Configure `YTUBE_API_TOKEN` (or `YT_API_TOKEN`) and `YOUTUBE_API_KEYS` to enable those fallback tiers; InnerTube remains first and yt-dlp remains the final fallback.

## Auto-leaving assistants

Telegram caps an account at 500 groups, so assistants leave chats that have gone quiet. A sweep runs 90 seconds after startup and hourly thereafter.

| Variable | Default | Description |
| --- | --- | --- |
| `AUTO_LEAVING_ASSISTANT` | `true` | Enable the idle-chat sweep |
| `ASSISTANT_LEAVE_TIME` | `5400` | Seconds a chat must be idle before it is left |
| `ASSISTANT_MAX_LEAVES_PER_SWEEP` | `10` | Cap per assistant per sweep; `0` means unlimited |
| `ASSISTANT_LEAVE_DRY_RUN` | `false` | Log every would-be leave without leaving |

A chat is kept whenever the answer is uncertain: chats with an active call, the `LOGGER_ID` chat, chats with authorized users, chats this process has never played in, and any chat whose authorization lookup fails. Because rejoining hundreds of groups requires fresh invite links, run a dry-run sweep first when changing these settings.

## Security

Direct media URLs resolving to loopback, private, link-local, multicast, or unspecified addresses are rejected by default. Only set `ALLOW_PRIVATE_STREAM_URLS=true` for a deliberately private deployment.

## Deployment

`app.json` declares the app for platforms that read it (Heroku, Render, etc.). It uses the `heroku/go` buildpack, a single `worker` process, and the same environment variables listed above. The NTgCalls native library is fetched at build time by `scripts/fetch-ntgcalls.sh` (see the Dockerfile), so no binary asset is committed.

Do not commit `.env`, session strings, bot tokens, or cookie files.
