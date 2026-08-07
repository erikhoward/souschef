package transcribe

import (
	"context"
	"errors"
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
func binAndModel(t *testing.T) (string, string, string) {
	t.Helper()

	bin := os.Getenv("WHISPER_BIN")
	if bin == "" {
		bin = "/opt/homebrew/bin/whisper-cli"
	}
	model := os.Getenv("WHISPER_MODEL")
	if model == "" {
		// Tests run with CWD = this package directory, so reach back to the
		// repo root where the README puts the model.
		model = filepath.Join("..", "..", "models", "ggml-base.en.bin")
	}
	ffmpeg := os.Getenv("FFMPEG_BIN")
	if ffmpeg == "" {
		ffmpeg = "/opt/homebrew/bin/ffmpeg"
	}

	if _, err := os.Stat(bin); err != nil {
		t.Skipf("whisper binary not found at %s — install with `brew install whisper-cpp`", bin)
	}
	if _, err := os.Stat(model); err != nil {
		t.Skipf("whisper model not found at %s — download a ggml model from "+
			"https://huggingface.co/ggerganov/whisper.cpp", model)
	}
	if _, err := os.Stat(ffmpeg); err != nil {
		t.Skipf("ffmpeg not found at %s — install with `brew install ffmpeg`", ffmpeg)
	}
	return bin, model, ffmpeg
}

// THE REGRESSION TEST. Telegram sends 48kHz Ogg/Opus. whisper.cpp cannot
// decode it: miniaudio fails the read, whisper prints "error: failed to read
// audio file" — and then EXITS 0. Before this fix that surfaced to the user as
// "Could not transcribe that: transcript was empty", with the real
// explanation discarded in stderr.
//
// This test is the one that would have caught it, and it could not have been
// written against the old fixture, which was a silent WAV.
func TestTranscribeDecodesTelegramOggOpus(t *testing.T) {
	bin, model, ffmpeg := binAndModel(t)
	tr := New(bin, model, ffmpeg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	got, err := tr.Transcribe(ctx, filepath.Join("testdata", "sample.oga"))
	if err != nil {
		t.Fatalf("an Ogg/Opus voice note must transcribe: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("transcript is empty — the Opus decode path is broken again")
	}
	// The fixture is the first 3 seconds of JFK's inaugural: "And so my fellow
	// Americans...". Asserting on real words rather than merely non-empty is
	// what makes this a decode test — an empty-string check would have passed
	// against the silent fixture this replaced.
	if !strings.Contains(strings.ToLower(got), "americans") {
		t.Errorf("transcript does not look like the fixture's speech: %q", got)
	}
	t.Logf("ogg transcript: %q", got)
}

// A format ffmpeg cannot decode must produce a legible error naming the file,
// not a silent empty transcript.
func TestTranscribeUndecodableInputIsLegible(t *testing.T) {
	bin, model, ffmpeg := binAndModel(t)
	tr := New(bin, model, ffmpeg)

	junk := filepath.Join(t.TempDir(), "not-audio.oga")
	if err := os.WriteFile(junk, []byte("this is not audio at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := tr.Transcribe(context.Background(), junk)
	if err == nil {
		t.Fatal("expected an error for undecodable input")
	}
	if errors.Is(err, ErrEmptyTranscript) {
		t.Error("a decode failure must not be reported as an empty transcript — that is the bug this fix exists for")
	}
	if !strings.Contains(err.Error(), "not-audio.oga") {
		t.Errorf("error should name the file, got: %v", err)
	}
}

func TestFirstWhisperError(t *testing.T) {
	stderr := "read_audio_data: trying to decode with miniaudio\n" +
		"read_audio_data: failed to read audio data\n" +
		"error: failed to read audio file 'voice.oga'\n"

	if got := firstWhisperError(stderr); got != "failed to read audio file 'voice.oga'" {
		t.Errorf("firstWhisperError = %q", got)
	}
	if got := firstWhisperError("all fine here\n"); got != "" {
		t.Errorf("no error line should yield empty, got %q", got)
	}
}

func TestTranscribeProducesText(t *testing.T) {
	bin, model, ffmpeg := binAndModel(t)
	tr := New(bin, model, ffmpeg)

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
	bin, model, ffmpeg := binAndModel(t)
	tr := New(bin, model, ffmpeg)

	_, err := tr.Transcribe(context.Background(), "testdata/does-not-exist.wav")
	if err == nil {
		t.Fatal("expected an error for a missing audio file")
	}
}

// The subprocess must not be able to hang the capture path indefinitely.
func TestTranscribeRespectsContextCancellation(t *testing.T) {
	bin, model, ffmpeg := binAndModel(t)
	tr := New(bin, model, ffmpeg)

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
