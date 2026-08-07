// Package config loads and validates every runtime prerequisite at startup.
//
// The design rule here is fail loud, fail early: a missing credential or an
// unreadable model file stops the process with a message naming the exact
// environment variable at fault. A previous iteration of this project spent
// hours diagnosing an enrichment hang that was really a dead API key, so
// nothing in this package logs a warning and carries on.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AnthropicKey   string
	Model          string
	Effort         string
	DBPath         string
	Port           int
	TelegramToken  string
	TelegramChatID int64
	WhisperBin     string
	WhisperModel   string
	FFmpegBin      string
	AudioDir       string

	// LoadedDotEnv reports whether a .env file in the working directory was
	// read. Callers surface this at startup so it is obvious which source the
	// configuration came from — a .env sitting unread next to a failing
	// process is exactly the confusion this field exists to prevent.
	LoadedDotEnv bool
}

// dotEnvFile is read from the process's working directory, which is why the
// README tells you to run the binary from the repo root: WHISPER_MODEL,
// SOUSCHEF_DB_PATH and AUDIO_DIR are all relative paths resolved the same way.
const dotEnvFile = ".env"

// loadDotEnv reads .env into the environment if it exists. Values already
// present in the environment WIN — godotenv.Load does not overwrite them —
// so an inline `SOUSCHEF_PORT=9999 ./bin/souschef` still overrides the file,
// which is what the test suite and the Playwright config rely on.
//
// A missing file is not an error: every automated run in this project supplies
// configuration inline. A file that exists but cannot be parsed IS an error —
// silently ignoring a typo'd .env would strand you exactly as an unread one does.
func loadDotEnv() (bool, error) {
	err := godotenv.Load(dotEnvFile)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("%s exists but could not be parsed: %w", dotEnvFile, err)
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment and verifies it. It returns
// every problem it finds at once rather than stopping at the first, so a fresh
// checkout surfaces all missing setup in a single run.
func Load() (Config, error) {
	loadedDotEnv, err := loadDotEnv()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		LoadedDotEnv:  loadedDotEnv,
		AnthropicKey:  os.Getenv("ANTHROPIC_API_KEY"),
		Model:         env("SOUSCHEF_MODEL", "claude-sonnet-5"),
		Effort:        env("SOUSCHEF_EFFORT", "low"),
		DBPath:        env("SOUSCHEF_DB_PATH", "./souschef.db"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		WhisperBin:    env("WHISPER_BIN", "/opt/homebrew/bin/whisper-cli"),
		WhisperModel:  env("WHISPER_MODEL", "./models/ggml-base.en.bin"),
		FFmpegBin:     env("FFMPEG_BIN", "/opt/homebrew/bin/ffmpeg"),
		AudioDir:      env("AUDIO_DIR", "./data/audio"),
	}

	var problems []string

	port, err := strconv.Atoi(env("SOUSCHEF_PORT", "8420"))
	if err != nil || port < 1 || port > 65535 {
		problems = append(problems, "SOUSCHEF_PORT must be a number between 1 and 65535")
	}
	cfg.Port = port

	// An unset ANTHROPIC_API_KEY is legitimate — the SDK also resolves an
	// `ant auth login` profile — so we cannot hard-fail here. The enrichment
	// path reports auth failures per-idea instead.

	if cfg.TelegramToken == "" {
		problems = append(problems, "TELEGRAM_BOT_TOKEN is required (get one from BotFather)")
	}

	rawChatID := os.Getenv("TELEGRAM_ALLOWED_CHAT_ID")
	if rawChatID == "" {
		problems = append(problems, "TELEGRAM_ALLOWED_CHAT_ID is required (message @userinfobot to find yours)")
	} else {
		id, err := strconv.ParseInt(rawChatID, 10, 64)
		if err != nil {
			problems = append(problems, "TELEGRAM_ALLOWED_CHAT_ID must be a numeric chat id")
		}
		cfg.TelegramChatID = id
	}

	if info, err := os.Stat(cfg.WhisperBin); err != nil {
		problems = append(problems, fmt.Sprintf(
			"WHISPER_BIN %q not found — install with `brew install whisper-cpp`", cfg.WhisperBin))
	} else if info.Mode()&0o111 == 0 {
		problems = append(problems, fmt.Sprintf("WHISPER_BIN %q is not executable", cfg.WhisperBin))
	}

	if _, err := os.Stat(cfg.WhisperModel); err != nil {
		problems = append(problems, fmt.Sprintf(
			"WHISPER_MODEL %q not found — download a ggml model from "+
				"https://huggingface.co/ggerganov/whisper.cpp", cfg.WhisperModel))
	}

	// ffmpeg is required, not optional. Telegram sends 48kHz Ogg/Opus and
	// whisper.cpp cannot decode it — it fails the read and then exits 0, so a
	// missing ffmpeg surfaces as a mysteriously empty transcript rather than
	// anything actionable. Catch it here instead.
	if info, err := os.Stat(cfg.FFmpegBin); err != nil {
		problems = append(problems, fmt.Sprintf(
			"FFMPEG_BIN %q not found — install with `brew install ffmpeg`. "+
				"It converts Telegram's Ogg/Opus voice notes to the 16kHz WAV whisper.cpp requires",
			cfg.FFmpegBin))
	} else if info.Mode()&0o111 == 0 {
		problems = append(problems, fmt.Sprintf("FFMPEG_BIN %q is not executable", cfg.FFmpegBin))
	}

	if err := os.MkdirAll(cfg.AudioDir, 0o755); err != nil {
		problems = append(problems, fmt.Sprintf("AUDIO_DIR %q could not be created: %v", cfg.AudioDir, err))
	}

	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration problems:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}
