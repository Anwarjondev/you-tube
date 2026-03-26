# YouTube Downloader Telegram Bot (Go)

This project is a Telegram bot written in Go. It accepts a YouTube URL, downloads the video using `yt-dlp`, and sends it back to the user.

## Requirements

- Go 1.24+
- `yt-dlp` installed and available in `PATH` (or set `YT_DLP_PATH`)
- `ffmpeg` installed (recommended for proper format merge)

## Setup

1. Copy `.env.example` to `.env`
2. Set `TELEGRAM_BOT_TOKEN`
3. Install dependencies and run:

```bash
go mod tidy
go run .
```

## Run with local Telegram Bot API (for large uploads)

1. Add these to `.env`:

```dotenv
TELEGRAM_API_ENDPOINT="http://127.0.0.1:8081/bot%s/%s"
TELEGRAM_API_ID=your_api_id
TELEGRAM_API_HASH=your_api_hash
```

2. Start local Bot API server:

```bash
./scripts/start-local-bot-api.sh
```

3. Run the bot:

```bash
go run .
```

If you get `connection refused` on `127.0.0.1:8081`, local Bot API is not running yet.
By default, when `TELEGRAM_API_ENDPOINT` points to local Bot API but local server is unavailable, the bot falls back to public API.
Set `REQUIRE_LOCAL_BOT_API=true` to enforce local-only mode and fail fast.

## Environment variables

- `TELEGRAM_BOT_TOKEN`: Telegram bot token (required)
- `TELEGRAM_API_ENDPOINT`: Optional API endpoint format (example: `http://localhost:8081/bot%s/%s`)
- `TELEGRAM_API_ID`: Telegram app API ID for local `telegram-bot-api` server
- `TELEGRAM_API_HASH`: Telegram app API hash for local `telegram-bot-api` server
- `REQUIRE_LOCAL_BOT_API`: Optional (`true`/`false`, default `false`). If `true`, bot exits when local endpoint is down.
- `MAX_VIDEO_SIZE_MB`: Max video size to process (default: `100000`, capped to endpoint-safe limit)
- `DOWNLOAD_TIMEOUT_SEC`: Download timeout in seconds (default: `300`)
- `LARGE_VIDEO_MODE`: `link` (default) downloads any size, uploads as video, falls back to file upload, then auto-splits into multiple parts if still too large; sends source URL only if upload paths fail. `reject` enforces size limit before upload
- `YT_DLP_PATH`: Optional full or relative path to `yt-dlp` binary
- `YT_DLP_JS_RUNTIMES`: Optional JS runtimes for yt-dlp (example: `node,deno`)
- `YT_DLP_COOKIES_FILE`: Optional cookies file for age-restricted/private videos

When `TELEGRAM_API_ENDPOINT` points to local Bot API (`localhost`/`127.0.0.1`), the bot uses local upload limits (up to local endpoint safety cap).

## Usage

- Start bot with `/start` or `/help`
- Send a YouTube link in chat

The bot will download any size. It tries to send as a video, then as a file, and if needed splits into Telegram-safe parts (with merge instructions).
