package transcribe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// binAndModel resolves the local whisper install, skipping the test when it
// is absent. whisper.cpp is a documented prerequisite, not a vendored
// dependency, so CI and a fresh checkout must not fail on its absence — but
// the skip message has to say exactly what to install.
func binAndModel(t *testing.T) (string, string) {
	t.Helper()

	bin := os.Getenv("WHISPER_BIN")
	if bin == "" {
		bin = "/opt/homebrew/bin/whisper-cli"
	}
	model := os.Getenv("WHISPER_MODEL")
	if model == "" {
		model = "./models/ggml-base.en.bin"
	}

	if _, err := os.Stat(bin); err != nil {
		t.Skipf("whisper binary not found at %s — install with `brew install whisper-cpp`", bin)
	}
	if _, err := os.Stat(model); err != nil {
		t.Skipf("whisper model not found at %s — download a ggml model from "+
			"https://huggingface.co/ggerganov/whisper.cpp", model)
	}
	return bin, model
}

func TestTranscribeProducesText(t *testing.T) {
	bin, model := binAndModel(t)
	tr := New(bin, model)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	got, err := tr.Transcribe(ctx, filepath.Join("testdata", "sample.wav"))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("transcript is empty")
	}
	t.Logf("transcript: %q", got)
}

func TestTranscribeMissingFileIsAnError(t *testing.T) {
	bin, model := binAndModel(t)
	tr := New(bin, model)

	_, err := tr.Transcribe(context.Background(), "testdata/does-not-exist.wav")
	if err == nil {
		t.Fatal("expected an error for a missing audio file")
	}
}

// The subprocess must not be able to hang the capture path indefinitely.
func TestTranscribeRespectsContextCancellation(t *testing.T) {
	bin, model := binAndModel(t)
	tr := New(bin, model)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	if _, err := tr.Transcribe(ctx, filepath.Join("testdata", "sample.wav")); err == nil {
		t.Fatal("expected an error when the context is already cancelled")
	}
}

// stripTimestamps is pure, so it is tested without the binary present.
func TestStripTimestamps(t *testing.T) {
	raw := "[00:00:00.000 --> 00:00:03.400]   sheet pan shawarma\n" +
		"[00:00:03.400 --> 00:00:05.100]   with lemony feta\n"

	got := stripTimestamps(raw)
	want := "sheet pan shawarma with lemony feta"
	if got != want {
		t.Errorf("stripTimestamps:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripTimestampsHandlesPlainOutput(t *testing.T) {
	if got := stripTimestamps("just plain text\n"); got != "just plain text" {
		t.Errorf("got %q", got)
	}
}
