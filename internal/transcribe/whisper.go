// Package transcribe converts voice notes to text by shelling out to
// whisper.cpp.
//
// This is local by design: Claude does not accept audio, so transcription is a
// separate component regardless, and running it locally costs nothing per
// note, has no quota to exhaust, and keeps recordings on the machine.
package transcribe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ErrEmptyTranscript = errors.New("transcript was empty")

// defaultTimeout bounds a single transcription. A voice note long enough to
// exceed this is not a capture, and the capture path must never hang.
const defaultTimeout = 3 * time.Minute

type Transcriber struct {
	bin     string
	model   string
	ffmpeg  string
	timeout time.Duration
}

// New builds a Transcriber. ffmpeg is required, not optional: Telegram sends
// Ogg/Opus and whisper.cpp cannot decode it, so every voice note goes through
// a conversion step.
func New(bin, model, ffmpeg string) *Transcriber {
	return &Transcriber{bin: bin, model: model, ffmpeg: ffmpeg, timeout: defaultTimeout}
}

// timestampLine matches whisper.cpp's default segment prefix.
var timestampLine = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\.\d{3} --> \d{2}:\d{2}:\d{2}\.\d{3}\]\s*`)

// stripTimestamps flattens whisper's segmented output into one line. It is
// pure so it can be tested without the binary installed.
func stripTimestamps(raw string) string {
	var parts []string
	for _, line := range strings.Split(raw, "\n") {
		line = timestampLine.ReplaceAllString(line, "")
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}

// whisperErrorLine matches the "error: ..." lines whisper.cpp writes to stderr.
//
// This exists because whisper.cpp EXITS 0 after a failed audio read. Telegram
// sends Ogg/Opus; whisper's miniaudio backend cannot decode it, prints
// "error: failed to read audio file", and then exits successfully. Trusting
// the exit status alone turns a hard decode failure into an empty transcript,
// which is exactly how this shipped broken: the user saw "transcript was
// empty" while the real explanation sat unread in stderr.
var whisperErrorLine = regexp.MustCompile(`(?m)^error:\s*(.+)$`)

// firstWhisperError returns the first "error:" line from stderr, or "".
func firstWhisperError(stderr string) string {
	if m := whisperErrorLine.FindStringSubmatch(stderr); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// Transcribe converts audioPath to text.
//
// Telegram voice notes arrive as 48kHz Ogg/Opus, which whisper.cpp cannot
// read, so every file is first normalised to 16kHz mono PCM WAV with ffmpeg.
// The conversion is unconditional rather than format-sniffed: ffmpeg is a
// no-op-ish pass for audio that is already correct, and one code path is
// easier to trust than two.
func (t *Transcriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	if _, err := os.Stat(audioPath); err != nil {
		return "", fmt.Errorf("audio file %s: %w", audioPath, err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	wavPath, cleanup, err := t.toWAV(ctx, audioPath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	// --no-prints keeps whisper's banner off stdout. We do NOT pass
	// --output-txt: it is a boolean flag, so giving it a value both diverts
	// the transcript to a sidecar .txt file and leaves the value stranded as a
	// bogus input path ("error: input file not found 'false'").
	cmd := exec.CommandContext(ctx, t.bin,
		"--model", t.model,
		"--file", wavPath,
		"--no-prints",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("transcription cancelled or timed out: %w", ctx.Err())
		}
		return "", fmt.Errorf("whisper failed: %w (stderr: %s)",
			err, strings.TrimSpace(stderr.String()))
	}

	// Exit status 0 is not proof of success — check stderr before trusting it.
	if msg := firstWhisperError(stderr.String()); msg != "" {
		return "", fmt.Errorf("whisper reported an error despite exiting cleanly: %s", msg)
	}

	text := stripTimestamps(stdout.String())
	if text == "" {
		return "", ErrEmptyTranscript
	}
	return text, nil
}

// toWAV normalises any input ffmpeg understands into the 16kHz mono PCM WAV
// whisper.cpp requires. It returns the converted path and a cleanup func the
// caller must always invoke.
func (t *Transcriber) toWAV(ctx context.Context, audioPath string) (string, func(), error) {
	tmp, err := os.CreateTemp("", "souschef-*.wav")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp wav: %w", err)
	}
	out := tmp.Name()
	tmp.Close()

	cleanup := func() { os.Remove(out) }

	cmd := exec.CommandContext(ctx, t.ffmpeg,
		"-nostdin",
		"-y",
		"-i", audioPath,
		"-ar", "16000", // whisper.cpp requires 16kHz
		"-ac", "1", // mono
		"-c:a", "pcm_s16le",
		"-f", "wav",
		out,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanup()
		if ctx.Err() != nil {
			return "", func() {}, fmt.Errorf("audio conversion cancelled or timed out: %w", ctx.Err())
		}
		return "", func() {}, fmt.Errorf("ffmpeg could not decode %s: %w (stderr: %s)",
			filepath.Base(audioPath), err, lastLines(stderr.String(), 3))
	}
	return out, cleanup, nil
}

// lastLines trims ffmpeg's verbose banner down to the part that explains a
// failure.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "; "))
}
