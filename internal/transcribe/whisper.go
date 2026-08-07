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
	timeout time.Duration
}

func New(bin, model string) *Transcriber {
	return &Transcriber{bin: bin, model: model, timeout: defaultTimeout}
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

// Transcribe runs whisper.cpp over audioPath and returns the flattened text.
func (t *Transcriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	if _, err := os.Stat(audioPath); err != nil {
		return "", fmt.Errorf("audio file %s: %w", audioPath, err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// --no-prints keeps whisper's banner off stdout; -nt would also drop the
	// timestamps, but we strip them ourselves so the parser still works if a
	// future version changes that flag.
	cmd := exec.CommandContext(ctx, t.bin,
		"--model", t.model,
		"--file", audioPath,
		"--output-txt", "false",
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

	text := stripTimestamps(stdout.String())
	if text == "" {
		return "", ErrEmptyTranscript
	}
	return text, nil
}
