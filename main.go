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
	defaultMaxVideoSizeMB       = 50
	defaultTimeoutSec           = 300
	defaultLargeVideoMode       = "reject"
	bytesPerMB            int64 = 1_000_000
	publicUploadLimitB    int64 = 2_000_000_000
	uploadSafetyMarginB   int64 = 1_000_000
)

type config struct {
	Token           string
	MaxSizeMB       int64
	MaxSizeB        int64
	MaxUploadLimitB int64
	TimeoutSec      int
	YtDlpPath       string
	Cookies         string
	JSRuntimes      string
	LargeVideoMode  string
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env not loaded, continuing with environment variables")
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}
	cfg.MaxUploadLimitB = safePublicUploadLimitB()
	if cfg.MaxSizeB > cfg.MaxUploadLimitB {
		cfg.MaxSizeB = cfg.MaxUploadLimitB
		cfg.MaxSizeMB = cfg.MaxUploadLimitB / bytesPerMB
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

		if _, err := bot.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatUploadVideo)); err != nil {
			log.Printf("chat action failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
		filePath, tempDir, err := downloadVideo(ctx, cfg, videoURL)
		cancel()
		if err != nil {
			text := fmt.Sprintf("Download failed: %v", err)
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
			continue
		}

		cleanup := func() {
			_ = os.RemoveAll(tempDir)
		}

		sizeBytes, err := fileSizeBytes(filePath)
		if err != nil {
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, "Could not read downloaded file size."))
			cleanup()
			continue
		}
		if sizeBytes > cfg.MaxSizeB {
			sizeMB := float64(sizeBytes) / float64(bytesPerMB)
			limitMB := float64(cfg.MaxSizeB) / float64(bytesPerMB)
			text := fmt.Sprintf("Video is too large (%.1f MB). Limit is %.0f MB.", sizeMB, limitMB)
			_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
			cleanup()
			continue
		}

		if err := uploadVideoWithFallback(bot, chatID, filePath); err != nil {
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
			"I currently support videos up to 50 MB.\n\n" +
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

	ytDlpPath, err := resolveYtDlpPath()
	if err != nil {
		return config{}, err
	}

	maxMB := getEnvInt64("MAX_VIDEO_SIZE_MB", defaultMaxVideoSizeMB)
	maxBytes := maxMB * bytesPerMB
	maxUploadLimitB := safePublicUploadLimitB()
	if maxBytes > maxUploadLimitB {
		maxBytes = maxUploadLimitB
	}
	timeoutSec := getEnvInt("DOWNLOAD_TIMEOUT_SEC", defaultTimeoutSec)
	largeVideoMode := strings.ToLower(strings.TrimSpace(os.Getenv("LARGE_VIDEO_MODE")))
	if largeVideoMode != "reject" && largeVideoMode != "link" {
		largeVideoMode = defaultLargeVideoMode
	}

	return config{
		Token:           token,
		MaxSizeMB:       maxBytes / bytesPerMB,
		MaxSizeB:        maxBytes,
		MaxUploadLimitB: maxUploadLimitB,
		TimeoutSec:      timeoutSec,
		YtDlpPath:       ytDlpPath,
		Cookies:         strings.TrimSpace(os.Getenv("YT_DLP_COOKIES_FILE")),
		JSRuntimes:      detectJSRuntimes(),
		LargeVideoMode:  largeVideoMode,
	}, nil
}

func safePublicUploadLimitB() int64 {
	return publicUploadLimitB - uploadSafetyMarginB
}

func detectJSRuntimes() string {
	if configured := strings.TrimSpace(os.Getenv("YT_DLP_JS_RUNTIMES")); configured != "" {
		return configured
	}

	var runtimes []string
	for _, runtime := range []string{"node", "deno", "bun"} {
		if _, err := exec.LookPath(runtime); err == nil {
			runtimes = append(runtimes, runtime)
		}
	}

	return strings.Join(runtimes, ",")
}

func resolveYtDlpPath() (string, error) {
	override := strings.TrimSpace(os.Getenv("YT_DLP_PATH"))
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("YT_DLP_PATH points to an invalid path: %w", err)
		}
		return override, nil
	}

	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path, nil
	}

	if _, err := os.Stat("./yt-dlp"); err == nil {
		return "./yt-dlp", nil
	}

	return "", errors.New("yt-dlp was not found. Install it or set YT_DLP_PATH to the binary")
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

func downloadVideo(ctx context.Context, cfg config, videoURL string) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "yt-bot-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}

	outputTemplate := filepath.Join(tmpDir, "%(title).80s.%(ext)s")
	args := []string{
		"--no-playlist",
		"--restrict-filenames",
		"--max-filesize", fmt.Sprintf("%dM", cfg.MaxSizeMB),
		"--extractor-args", "youtube:player_client=android,web",
		"-f", "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4]/b",
		"--merge-output-format", "mp4",
		"-o", outputTemplate,
	}

	if cfg.JSRuntimes != "" {
		args = append(args, "--js-runtimes", cfg.JSRuntimes)
	}
	if cfg.Cookies != "" {
		args = append(args, "--cookies", cfg.Cookies)
	}
	args = append(args, videoURL)

	cmd := exec.CommandContext(ctx, cfg.YtDlpPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		if ctx.Err() != nil {
			return "", "", errors.New("download timed out")
		}
		return "", "", normalizeYtDlpError(string(out))
	}

	filePath, err := findMainVideoFile(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", err
	}

	return filePath, tmpDir, nil
}

func normalizeYtDlpError(raw string) error {
	msg := strings.TrimSpace(raw)
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "this video is not available") || strings.Contains(lower, "video unavailable") {
		return errors.New("video is unavailable, private, or region-restricted")
	}
	if strings.Contains(lower, "sign in to confirm your age") || strings.Contains(lower, "this content may be inappropriate") {
		return errors.New("age-restricted video; set YT_DLP_COOKIES_FILE with exported YouTube cookies")
	}
	if strings.Contains(lower, "no supported javascript runtime could be found") {
		return errors.New("YouTube requires a JavaScript runtime; install node/deno or set YT_DLP_JS_RUNTIMES")
	}

	const prefix = "ERROR:"
	if idx := strings.LastIndex(msg, prefix); idx >= 0 {
		return fmt.Errorf("yt-dlp error: %s", strings.TrimSpace(msg[idx+len(prefix):]))
	}

	if msg == "" {
		return errors.New("yt-dlp failed with no output")
	}

	return fmt.Errorf("yt-dlp error: %s", msg)
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

func fileSizeBytes(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func uploadVideoWithFallback(bot *tgbotapi.BotAPI, chatID int64, filePath string) error {
	video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))
	video.Caption = "Downloaded successfully"
	_, err := bot.Send(video)
	return err
}
