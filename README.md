# YouTube Downloader Telegram Bot (Go)

This project is a Telegram bot written in Go. It accepts a YouTube URL, downloads the video using `yt-dlp`, and sends it back to the user.

## Requirements

- Go 1.24+
- `yt-dlp` installed and available in `PATH`
- `ffmpeg` installed (recommended for proper format merge)

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
- `MAX_VIDEO_SIZE_MB`: Max upload size limit (default: `45`)
- `DOWNLOAD_TIMEOUT_SEC`: Download timeout in seconds (default: `300`)

## Usage

- Start bot with `/start` or `/help`
- Send a YouTube link in chat

The bot will download and upload the video file if it fits the configured size limit.
