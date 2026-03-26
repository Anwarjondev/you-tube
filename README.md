# YouTube Downloader Telegram Bot (Go)

This project is a Telegram bot written in Go. It accepts a YouTube URL, downloads the video using `yt-dlp`, and sends it back to the user.

## Requirements

- Go 1.24+
- `yt-dlp` installed and available in `PATH` (or set `YT_DLP_PATH`)
- `ffmpeg` installed (recommended for proper format merge)

## Install yt-dlp

Choose one method for your OS.

### Linux

Using `pip`:

```bash
python3 -m pip install -U yt-dlp
```

Or download binary directly:

```bash
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod a+rx /usr/local/bin/yt-dlp
```

### macOS

Using Homebrew:

```bash
brew install yt-dlp
```

### Windows

Using `winget`:

```powershell
winget install yt-dlp.yt-dlp
```

Or using `pip`:

```powershell
py -m pip install -U yt-dlp
```

Verify installation:

```bash
yt-dlp --version
```

## Setup

1. Copy `.env.example` to `.env`
2. Set `TELEGRAM_BOT_TOKEN`
3. Install dependencies and run:

```bash
go mod tidy
go run .
```

## Environment variables

- `TELEGRAM_BOT_TOKEN`: Telegram bot token (required)
- `MAX_VIDEO_SIZE_MB`: Max video size to process (default: `50`)
- `DOWNLOAD_TIMEOUT_SEC`: Download timeout in seconds (default: `300`)
- `YT_DLP_PATH`: Optional full or relative path to `yt-dlp` binary
- `YT_DLP_JS_RUNTIMES`: Optional JS runtimes for yt-dlp (example: `node,deno`)
- `YT_DLP_COOKIES_FILE`: Optional cookies file for age-restricted/private videos

The bot is intentionally simple: it supports videos up to the configured limit (50 MB by default).

## Usage

- Start bot with `/start` or `/help`
- Send a YouTube link in chat

The bot downloads and sends the video directly. If the file is over the configured size limit, it returns a clear "too large" message.
