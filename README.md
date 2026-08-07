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

```bash
brew install whisper-cpp
mkdir -p models
curl -fsSL -o models/ggml-base.en.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin
```

## Setup

```bash
cp .env.example .env   # then fill in the values
make build
./bin/souschef
```

The app refuses to start if anything is missing, and names what. Open
http://localhost:8420.

## Development

```bash
make dev    # Go on :8420, Vite on :5173 with hot reload
make test   # Go tests plus Playwright
```

## Telegram

The token is all BotFather needs to provide — **the app publishes its own
command list** on startup. Send it a message or a voice note to capture an
idea, and `/s <query>` to search.
