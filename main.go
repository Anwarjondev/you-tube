package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

const (
	defaultMaxVideoSizeMB = 45
	defaultTimeoutSec     = 300
)

type config struct {
	Token      string
	MaxSizeMB  int64
	TimeoutSec int
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env not loaded, continuing with environment variables")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := exec.LookPath("./yt-dlp"); err != nil {
		log.Fatal("yt-dlp was not found in PATH. Install it first.")
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	log.Printf("authorized on account: %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			handleCommand(bot, update.Message)
			continue
		}

		raw := strings.TrimSpace(update.Message.Text)
		videoURL, err := extractYouTubeURL(raw)
		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Send a valid YouTube link. Example: https://youtu.be/dQw4w9WgXcQ")
			_, _ = bot.Send(msg)
			continue
		}

		chatID := update.Message.Chat.ID
		statusMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, "Downloading video. Please wait..."))

		if _, err := bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatUploadVideo)); err != nil {
			log.Printf("chat action failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
		filePath, tempDir, err := downloadVideo(ctx, videoURL)
		cancel()
		if err != nil {
			text := fmt.Sprintf("Download failed: %v", err)
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
			continue
		}

		cleanup := func() {
			_ = os.RemoveAll(tempDir)
		}

		size, err := fileSizeMB(filePath)
		if err != nil {
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, "Could not read downloaded file size."))
			cleanup()
			continue
		}
		if size > cfg.MaxSizeMB {
			text := fmt.Sprintf("Video is too large (%.1f MB). Limit is %d MB.", float64(size), cfg.MaxSizeMB)
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
			cleanup()
			continue
		}

		video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))
		video.Caption = "Downloaded successfully"
		if _, err := bot.Send(video); err != nil {
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Upload failed: %v", err)))
			cleanup()
			continue
		}

		cleanup()

		if statusMsg.MessageID != 0 {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID)
			_, _ = bot.Request(deleteMsg)
		}
	}
}

func handleCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start", "help":
		help := "Send me a YouTube link and I will download and send back the video file.\n\n" +
			"Requirements on server:\n" +
			"- yt-dlp installed\n" +
			"- ffmpeg installed (recommended for merge/format conversions)"
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, help))
	default:
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "Unknown command. Use /help"))
	}
}

func loadConfig() (config, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	maxMB := getEnvInt64("MAX_VIDEO_SIZE_MB", defaultMaxVideoSizeMB)
	timeoutSec := getEnvInt("DOWNLOAD_TIMEOUT_SEC", defaultTimeoutSec)

	return config{
		Token:      token,
		MaxSizeMB:  maxMB,
		TimeoutSec: timeoutSec,
	}, nil
}

func getEnvInt(name string, defaultValue int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

func getEnvInt64(name string, defaultValue int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultValue
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

func extractYouTubeURL(text string) (string, error) {
	parts := strings.Fields(text)
	for _, part := range parts {
		u, err := url.Parse(part)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
			return part, nil
		}
	}
	return "", errors.New("no YouTube URL found")
}

func downloadVideo(ctx context.Context, videoURL string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "yt-bot-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}

	outputTemplate := filepath.Join(tmpDir, "%(title).80s.%(ext)s")
	args := []string{
		"--no-playlist",
		"--restrict-filenames",
		"-f", "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4]/b",
		"--merge-output-format", "mp4",
		"-o", outputTemplate,
		videoURL,
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		if ctx.Err() != nil {
			return "", "", errors.New("download timed out")
		}
		return "", "", fmt.Errorf("yt-dlp error: %s", strings.TrimSpace(string(out)))
	}

	filePath, err := findMainVideoFile(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", err
	}

	return filePath, tmpDir, nil
}

func findMainVideoFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read temp dir: %w", err)
	}

	var (
		bestPath string
		bestSize int64
	)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if !(strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mkv") || strings.HasSuffix(name, ".webm")) {
			continue
		}
		full := filepath.Join(dir, e.Name())
		fi, err := os.Stat(full)
		if err != nil {
			continue
		}
		if fi.Size() > bestSize {
			bestSize = fi.Size()
			bestPath = full
		}
	}

	if bestPath == "" {
		return "", errors.New("download finished but no video file was found")
	}

	return bestPath, nil
}

func fileSizeMB(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size() / (1024 * 1024), nil
}
