package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{
		"ANTHROPIC_API_KEY", "SOUSCHEF_MODEL", "SOUSCHEF_EFFORT", "SOUSCHEF_DB_PATH",
		"SOUSCHEF_PORT", "TELEGRAM_BOT_TOKEN", "TELEGRAM_ALLOWED_CHAT_ID",
		"WHISPER_BIN", "WHISPER_MODEL", "AUDIO_DIR",
	} {
		os.Unsetenv(k)
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// validEnv returns an environment that should load cleanly, with real files
// on disk for the whisper binary and model.
func validEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "whisper-cli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "ggml-base.en.bin")
	if err := os.WriteFile(model, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"ANTHROPIC_API_KEY":        "sk-ant-test",
		"SOUSCHEF_DB_PATH":         filepath.Join(dir, "test.db"),
		"TELEGRAM_BOT_TOKEN":       "123:abc",
		"TELEGRAM_ALLOWED_CHAT_ID": "42",
		"WHISPER_BIN":              bin,
		"WHISPER_MODEL":            model,
		"AUDIO_DIR":                filepath.Join(dir, "audio"),
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	setEnv(t, validEnv(t))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected clean load, got %v", err)
	}
	if cfg.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", cfg.Model)
	}
	if cfg.Effort != "low" {
		t.Errorf("Effort = %q, want low", cfg.Effort)
	}
	if cfg.Port != 8420 {
		t.Errorf("Port = %d, want 8420", cfg.Port)
	}
	if cfg.TelegramChatID != 42 {
		t.Errorf("TelegramChatID = %d, want 42", cfg.TelegramChatID)
	}
}

func TestLoadFailsOnMissingWhisperBinary(t *testing.T) {
	env := validEnv(t)
	env["WHISPER_BIN"] = "/nonexistent/whisper-cli"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing whisper binary")
	}
	if !strings.Contains(err.Error(), "WHISPER_BIN") {
		t.Errorf("error must name the offending variable, got: %v", err)
	}
}

func TestLoadFailsOnMissingTelegramToken(t *testing.T) {
	env := validEnv(t)
	delete(env, "TELEGRAM_BOT_TOKEN")
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing telegram token")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") {
		t.Errorf("error must name the offending variable, got: %v", err)
	}
}

func TestLoadFailsOnNonNumericChatID(t *testing.T) {
	env := validEnv(t)
	env["TELEGRAM_ALLOWED_CHAT_ID"] = "not-a-number"
	setEnv(t, env)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-numeric chat id")
	}
}

func TestLoadCreatesAudioDir(t *testing.T) {
	env := validEnv(t)
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected clean load, got %v", err)
	}
	info, err := os.Stat(cfg.AudioDir)
	if err != nil || !info.IsDir() {
		t.Errorf("AudioDir %q should exist as a directory", cfg.AudioDir)
	}
}
