# Sous Chef

A local-first pipeline for capturing recipe and video ideas, inferring their
metadata with Claude, and organising the backlog. Capture from a web app or
from Telegram, by text or by voice.

## Prerequisites

- Go 1.26+
- Bun 1.3+
- An Anthropic credential — either `ANTHROPIC_API_KEY` or an `ant auth login` profile
- A Telegram bot token from [@BotFather](https://t.me/botfather)
- whisper.cpp, for voice notes:

Run these from the repository root — `models/` is expected there, and it is
gitignored:

```bash
brew install whisper-cpp
mkdir -p models
curl -fsSL -o models/ggml-base.en.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin
```

The download is about 148MB. Homebrew installs the binary as
`/opt/homebrew/bin/whisper-cli`, which is the default `WHISPER_BIN`.

Note that whisper.cpp gates **startup**, not just voice notes: the app
verifies the binary and the model exist before it will run at all.

## Setup

```bash
cp .env.example .env   # then fill in the values
make build
./bin/souschef
```

**Run the binary from the repository root.** `.env` is read from the working
directory, and `WHISPER_MODEL`, `SOUSCHEF_DB_PATH` and `AUDIO_DIR` default to
paths relative to it. Starting from anywhere else means the app will not find
your configuration.

Values already set in the environment take precedence over `.env`, so you can
override one for a single run without editing the file:

```bash
SOUSCHEF_PORT=9000 ./bin/souschef
```

The app refuses to start if anything is missing, and names what. It also logs
on the first line whether it read a `.env` or fell back to the ambient
environment. Open http://localhost:8420.

## Development

```bash
make dev    # Go on :8420, Vite on :5173 with hot reload
make test   # Go tests plus Playwright
```

## Telegram

The token is all BotFather needs to provide — **the app publishes its own
command list** on startup. Send it a message or a voice note to capture an
idea, and `/s <query>` to search.
