# Sous Chef Milestone 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture recipe and video ideas instantly from a web app or Telegram (text or voice), have Claude infer their metadata in the background, and organize the resulting backlog.

**Architecture:** A single Go binary serves a REST API and the built React app from `embed.FS`. Capture writes to SQLite and returns immediately; a goroutine then calls Claude and pushes the enriched row to the browser over Server-Sent Events. Telegram uses outbound long polling, so no public URL is needed. Only `internal/store` writes SQL; `internal/enrich` is a pure function from text to metadata and never touches the database.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure Go, FTS5 built in — no cgo), `github.com/anthropics/anthropic-sdk-go`, `github.com/google/uuid`, React 19 + Vite 8, Playwright. Telegram is a hand-rolled ~150-line client over `net/http` — we need seven Bot API methods and would otherwise inherit an SDK's release cadence for no gain.

**Spec:** `docs/superpowers/specs/2026-08-06-souschef-capture-design.md`

## Global Constraints

- Module path is `github.com/erikhoward/souschef`.
- **Only `internal/store` writes SQL.** Every other package goes through it.
- **`internal/enrich` never touches the database.** It takes text, returns metadata or an error.
- Model default is `claude-sonnet-5`, read from `SOUSCHEF_MODEL`. Never hardcode a model ID outside `config`.
- Use `output_config.format` with a JSON schema for structured output. The top-level `output_format` parameter is deprecated — do not use it.
- Use `thinking: {type: "adaptive"}`. Never send `budget_tokens`, `temperature`, `top_p`, or `top_k` — all four return 400 on Sonnet 5.
- Enrichment failures are **never silent.** Every failure writes verbatim error text to `ideas.enrichment_error` and sets `enrichment_status='failed'`.
- Archive is reversible and **must preserve `stage`.** Delete is permanent.
- Palette and type are CSS custom properties on `:root`. Accent is `#4f6b4a`. Fonts are self-hosted woff2 — no CDN.
- Every task ends on a green test run and a commit.
- Branch: `feat/milestone-1-capture`. Do not commit to `main`.

## Phases

| Phase | Tasks | Deliverable |
|---|---|---|
| 0 — Foundation | 1–2 | Restructured repo, config that fails fast |
| 1 — Persistence | 3–6 | SQLite with FTS5, full domain logic |
| 2 — Intelligence | 7 | Claude enrichment, fixture-tested offline |
| 3 — Serving | 8–10 | REST + SSE + single binary |
| 4 — Interface | 11–13 | Retheme, live data, correction UI |
| 5 — Telegram | 14–17 | Voice, capture, tappable search, e2e |

---

## File Structure

**Created:**

| Path | Responsibility |
|---|---|
| `cmd/souschef/main.go` | Wire config → store → services → HTTP + Telegram; graceful shutdown |
| `internal/config/config.go` | Env loading, validation, fail-fast startup checks |
| `internal/store/store.go` | Connection, migration runner |
| `internal/store/ideas.go` | Idea row read/write, list filtering and sorting |
| `internal/store/search.go` | FTS5 query |
| `internal/store/relations.go` | Notes, tags, links, merge |
| `internal/ideas/idea.go` | Domain types and enum constants |
| `internal/ideas/service.go` | Create, correct, archive, restore, link, merge rules |
| `internal/enrich/enrich.go` | Claude call, schema, error classification |
| `internal/enrich/prompt.go` | Taxonomy system prompt |
| `internal/transcribe/whisper.go` | whisper.cpp subprocess wrapper |
| `internal/telegram/client.go` | Bot API HTTP client |
| `internal/telegram/commands.go` | Command registry — single source of truth |
| `internal/telegram/bot.go` | Long-poll loop, routing, handlers |
| `internal/httpapi/router.go` | Routes, JSON encoding, embedded assets |
| `internal/httpapi/sse.go` | Subscriber hub |
| `migrations/0001_init.sql` | Schema + FTS5 triggers |
| `web/src/hooks/useIdeas.js` | Fetch, optimistic insert, SSE reconnect |
| `web/src/lib/api.js` | Typed fetch wrappers |

**Moved:** `index.html`, `package.json`, `bun.lock`, `public/`, `src/`, `tests/` → under `web/`.

**Deleted:** `web/src/lib/pipeline.js`, `web/src/data/ideas.js`, `web/src/components/RecipeWorkspace.jsx`, `web/tests/pipeline.test.js`, `web/src/assets/recipe-thumbnails.png`.

---

## Task 1: Repo restructure and Go module

**Files:**
- Create: `go.mod`, `cmd/souschef/main.go`, `cmd/souschef/main_test.go`, `Makefile`, `.env.example`
- Move: `index.html`, `package.json`, `bun.lock`, `public/`, `src/`, `tests/` → `web/`

**Interfaces:**
- Consumes: nothing.
- Produces: module `github.com/erikhoward/souschef`; `main.version` string; a working `make build`, `make dev`, `make test`.

- [ ] **Step 1: Create the branch**

```bash
git checkout -b feat/milestone-1-capture
```

- [ ] **Step 2: Move the web app with git mv so history follows**

```bash
mkdir -p web
git mv index.html package.json bun.lock public src tests web/
git mv .gitignore .gitignore.tmp && git mv .gitignore.tmp .gitignore  # no-op, keeps ignore at root
ls web/
```

Expected: `bun.lock  index.html  package.json  public  src  tests`

- [ ] **Step 3: Verify the web app still builds from its new home**

```bash
cd web && bun run build && cd ..
```

Expected: `✓ built in ...ms`. If Vite complains about a missing root, that is expected — it resolves because `index.html` moved alongside `package.json`.

- [ ] **Step 4: Initialise the Go module**

```bash
go mod init github.com/erikhoward/souschef
```

- [ ] **Step 5: Write the failing test**

Create `cmd/souschef/main_test.go`:

```go
package main

import "testing"

func TestVersionIsSet(t *testing.T) {
	if version == "" {
		t.Fatal("version must not be empty")
	}
}
```

- [ ] **Step 6: Run it to make sure it fails**

Run: `go test ./cmd/souschef/ -run TestVersionIsSet -v`
Expected: FAIL — `undefined: version`

- [ ] **Step 7: Write the minimal implementation**

Create `cmd/souschef/main.go`:

```go
package main

import "fmt"

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	fmt.Printf("souschef %s\n", version)
}
```

- [ ] **Step 8: Run the test to make sure it passes**

Run: `go test ./cmd/souschef/ -run TestVersionIsSet -v`
Expected: PASS

- [ ] **Step 9: Write the Makefile**

Create `Makefile` (tabs, not spaces, for recipe lines):

```makefile
.PHONY: dev build test clean

dev:
	@echo "Starting Go API on :8420 and Vite on :5173"
	@(cd web && bun run dev) & go run ./cmd/souschef

build:
	cd web && bun install --frozen-lockfile && bun run build
	go build -ldflags "-X main.version=$$(git describe --tags --always --dirty)" -o bin/souschef ./cmd/souschef
	@echo "Built bin/souschef"

test:
	go test ./... -race
	cd web && bunx playwright test

clean:
	rm -rf bin web/dist
```

- [ ] **Step 10: Verify build and test targets run**

```bash
make build && ./bin/souschef
```

Expected: `Built bin/souschef` then `souschef <git-sha>`

- [ ] **Step 11: Write .env.example**

Create `.env.example`:

```
# Anthropic — or leave unset and use an `ant auth login` profile
ANTHROPIC_API_KEY=
SOUSCHEF_MODEL=claude-sonnet-5
SOUSCHEF_EFFORT=low

SOUSCHEF_DB_PATH=./souschef.db
SOUSCHEF_PORT=8420

# Telegram — token from BotFather. Commands are published by this app, not BotFather.
TELEGRAM_BOT_TOKEN=
TELEGRAM_ALLOWED_CHAT_ID=

# whisper.cpp — see README for install
WHISPER_BIN=/opt/homebrew/bin/whisper-cli
WHISPER_MODEL=./models/ggml-base.en.bin
AUDIO_DIR=./data/audio
```

- [ ] **Step 12: Add bin/ to .gitignore**

Append to `.gitignore`:

```
/bin/
```

- [ ] **Step 13: Commit**

```bash
git add -A
git commit -m "refactor: move web app into web/, add Go module and Makefile

Makes room for the backend beside the frontend. git mv preserves history
on the moved files. Adds a version-stamped build and .env.example.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Config loading and fail-fast validation

This is the direct fix for the prior incident where a dead key surfaced as an indefinite hang. Every prerequisite is checked at boot.

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `cmd/souschef/main.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Config` struct; `config.Load() (Config, error)`; `config.ErrMissing` sentinel. Fields used by later tasks: `DBPath`, `Port`, `Model`, `Effort`, `AnthropicKey`, `TelegramToken`, `TelegramChatID int64`, `WhisperBin`, `WhisperModel`, `AudioDir`.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:

```go
// Package config loads and validates every runtime prerequisite at startup.
//
// The design rule here is fail loud, fail early: a missing credential or an
// unreadable model file stops the process with a message naming the exact
// environment variable at fault. A previous iteration of this project spent
// hours diagnosing an enrichment hang that was really a dead API key, so
// nothing in this package logs a warning and carries on.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	AudioDir       string
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
	cfg := Config{
		AnthropicKey:  os.Getenv("ANTHROPIC_API_KEY"),
		Model:         env("SOUSCHEF_MODEL", "claude-sonnet-5"),
		Effort:        env("SOUSCHEF_EFFORT", "low"),
		DBPath:        env("SOUSCHEF_DB_PATH", "./souschef.db"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		WhisperBin:    env("WHISPER_BIN", "/opt/homebrew/bin/whisper-cli"),
		WhisperModel:  env("WHISPER_MODEL", "./models/ggml-base.en.bin"),
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

	if err := os.MkdirAll(cfg.AudioDir, 0o755); err != nil {
		problems = append(problems, fmt.Sprintf("AUDIO_DIR %q could not be created: %v", cfg.AudioDir, err))
	}

	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration problems:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run the tests to make sure they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — all five tests

- [ ] **Step 5: Wire config into main**

Replace `cmd/souschef/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/erikhoward/souschef/internal/config"
)

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "souschef %s failed to start.\n\n%v\n", version, err)
		os.Exit(1)
	}
	log.Printf("souschef %s — db=%s port=%d model=%s", version, cfg.DBPath, cfg.Port, cfg.Model)
}
```

- [ ] **Step 6: Verify the failure path is legible**

```bash
env -u TELEGRAM_BOT_TOKEN -u TELEGRAM_ALLOWED_CHAT_ID go run ./cmd/souschef; echo "exit=$?"
```

Expected: a `configuration problems:` block naming `TELEGRAM_BOT_TOKEN`, `TELEGRAM_ALLOWED_CHAT_ID`, and the whisper paths, then `exit=1`.

- [ ] **Step 7: Run the whole suite**

Run: `go test ./... -race`
Expected: PASS (`cmd/souschef`, `internal/config`)

- [ ] **Step 8: Commit**

```bash
git add internal/config cmd/souschef
git commit -m "feat(config): load and validate every prerequisite at startup

Reports all problems at once, each naming the offending variable, and
refuses to boot rather than failing later. ANTHROPIC_API_KEY is
deliberately not required — the SDK also resolves an ant auth profile,
so auth failures surface per-idea on the enrichment path instead.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: SQLite connection, migration runner, and schema

**Files:**
- Create: `internal/store/store.go`, `internal/store/store_test.go`, `migrations/0001_init.sql`

**Interfaces:**
- Consumes: `config.Config.DBPath`.
- Produces: `store.Store` struct wrapping `*sql.DB`; `store.Open(path string) (*Store, error)`; `(*Store).Close() error`; `(*Store).DB() *sql.DB`. Migrations are embedded and applied by `Open`.

- [ ] **Step 1: Add dependencies**

```bash
go get modernc.org/sqlite@latest
go get github.com/google/uuid@latest
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

// newTestStore opens a store in a temp file. We deliberately do not use
// :memory: — FTS5 external-content triggers must survive a close and reopen,
// and an in-memory database would hide a broken trigger definition.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := newTestStore(t)

	var count int
	err := s.DB().QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ideas'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatal("ideas table was not created")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopening an already-migrated database must succeed: %v", err)
	}
	defer s2.Close()

	var applied int
	if err := s2.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("query: %v", err)
	}
	if applied != 1 {
		t.Errorf("schema_migrations rows = %d, want 1 (migration applied twice?)", applied)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := newTestStore(t)

	_, err := s.DB().Exec(`INSERT INTO notes (id, idea_id, body, created_at)
	                       VALUES ('n1', 'does-not-exist', 'orphan', datetime('now'))`)
	if err == nil {
		t.Fatal("expected foreign key violation for note with unknown idea_id")
	}
}

func TestFTSTableExists(t *testing.T) {
	s := newTestStore(t)

	var count int
	err := s.DB().QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE name='ideas_fts'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count == 0 {
		t.Fatal("ideas_fts virtual table was not created")
	}
}
```

- [ ] **Step 3: Run it to make sure it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL — `undefined: Open`

- [ ] **Step 4: Write the migration**

Create `migrations/0001_init.sql`:

```sql
CREATE TABLE ideas (
    id                 TEXT PRIMARY KEY,
    title              TEXT NOT NULL,
    raw_text           TEXT NOT NULL,
    source             TEXT NOT NULL,
    source_ref         TEXT,
    stage              TEXT NOT NULL DEFAULT 'idea',
    archived_at        DATETIME,
    merged_into_id     TEXT REFERENCES ideas(id) ON DELETE SET NULL,

    difficulty         TEXT,
    duration_class     TEXT,
    treatment          TEXT,
    content_type       TEXT,
    cuisine            TEXT,
    primary_ingredient TEXT,
    equipment          TEXT,
    visual_potential   TEXT,
    seasonality        TEXT,
    production_effort  TEXT,
    field_overrides    TEXT NOT NULL DEFAULT '[]',

    enrichment_status  TEXT NOT NULL DEFAULT 'pending',
    enrichment_error   TEXT,
    enrichment_model   TEXT,
    enriched_at        DATETIME,

    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL
);

CREATE INDEX idx_ideas_stage      ON ideas(stage);
CREATE INDEX idx_ideas_archived   ON ideas(archived_at);
CREATE INDEX idx_ideas_created    ON ideas(created_at DESC);
CREATE INDEX idx_ideas_enrichment ON ideas(enrichment_status);

CREATE TABLE notes (
    id         TEXT PRIMARY KEY,
    idea_id    TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_notes_idea ON notes(idea_id);

CREATE TABLE tags (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE idea_tags (
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (idea_id, tag_id)
);

-- Links are symmetric and stored once. The CHECK enforces canonical ordering
-- so (a,b) and (b,a) cannot both exist, and a self-link is impossible.
CREATE TABLE idea_links (
    idea_a_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    idea_b_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    PRIMARY KEY (idea_a_id, idea_b_id),
    CHECK (idea_a_id < idea_b_id)
);

CREATE TABLE transcripts (
    id          TEXT PRIMARY KEY,
    idea_id     TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    audio_path  TEXT NOT NULL,
    text        TEXT NOT NULL,
    duration_ms INTEGER,
    created_at  DATETIME NOT NULL
);

-- Contentless-adjacent FTS: we store the searchable text directly so tags,
-- which live in a join table, can be denormalised into the index.
CREATE VIRTUAL TABLE ideas_fts USING fts5(
    id UNINDEXED,
    title,
    raw_text,
    cuisine,
    primary_ingredient,
    tags,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER ideas_fts_insert AFTER INSERT ON ideas BEGIN
    INSERT INTO ideas_fts (id, title, raw_text, cuisine, primary_ingredient, tags)
    VALUES (new.id, new.title, new.raw_text,
            coalesce(new.cuisine, ''), coalesce(new.primary_ingredient, ''), '');
END;

CREATE TRIGGER ideas_fts_update AFTER UPDATE ON ideas BEGIN
    UPDATE ideas_fts
       SET title              = new.title,
           raw_text           = new.raw_text,
           cuisine            = coalesce(new.cuisine, ''),
           primary_ingredient = coalesce(new.primary_ingredient, '')
     WHERE id = new.id;
END;

CREATE TRIGGER ideas_fts_delete AFTER DELETE ON ideas BEGIN
    DELETE FROM ideas_fts WHERE id = old.id;
END;
```

- [ ] **Step 5: Write the store**

Create `internal/store/store.go`:

```go
// Package store owns every SQL statement in the application. No other package
// writes queries — that boundary is what keeps the web and Telegram surfaces
// from drifting on what operations like "archive" actually mean.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed all:../../migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db *sql.DB
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error { return s.db.Close() }

// Open connects to the database at path, enables the pragmas we depend on,
// and applies any pending migrations.
func Open(path string) (*Store, error) {
	// WAL keeps the background enrichment writer from blocking API reads.
	// Foreign keys are off by default in SQLite and must be asked for.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name       TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("../../migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := s.db.QueryRow(
			`SELECT count(*) FROM schema_migrations WHERE name = ?`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("../../migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}
	return nil
}
```

- [ ] **Step 6: Run the tests to make sure they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS — all four tests.

If `//go:embed all:../../migrations/*.sql` errors with "pattern outside module", move the embed to a small `migrations/migrations.go` declaring `package migrations` with `//go:embed *.sql` and have `store` import it. Prefer that shape if the relative embed misbehaves.

- [ ] **Step 7: Commit**

```bash
git add internal/store migrations go.mod go.sum
git commit -m "feat(store): SQLite connection, migration runner, and schema

WAL so background enrichment writes don't block API reads, foreign keys
on (off by default in SQLite), and an FTS5 index over title, raw text,
cuisine and ingredient. Links use a CHECK on canonical ordering so
symmetry and self-link rejection are enforced by the database.

Tests use a temp file rather than :memory: — FTS triggers must survive a
close and reopen, which an in-memory database would not exercise.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Idea row persistence — insert, get, list

**Files:**
- Create: `internal/ideas/idea.go`, `internal/store/ideas.go`, `internal/store/ideas_test.go`

**Interfaces:**
- Consumes: `store.Store` from Task 3.
- Produces:
  - `ideas.Idea`, `ideas.Metadata`, `ideas.Enrichment` structs
  - constants `ideas.StageIdea`, `ideas.SourceWeb`, `ideas.SourceTelegramText`, `ideas.SourceTelegramVoice`, `ideas.EnrichPending`, `ideas.EnrichOK`, `ideas.EnrichFailed`
  - `ideas.ListFilter` struct
  - `(*Store).InsertIdea(ctx, ideas.Idea) error`
  - `(*Store).GetIdea(ctx, id string) (ideas.Idea, error)`
  - `(*Store).ListIdeas(ctx, ideas.ListFilter) ([]ideas.Idea, error)`
  - `(*Store).UpdateIdea(ctx, ideas.Idea) error`
  - `(*Store).DeleteIdea(ctx, id string) error`
  - `store.ErrNotFound`

- [ ] **Step 1: Write the domain types**

Create `internal/ideas/idea.go`:

```go
// Package ideas holds the domain model and the rules that govern it. It knows
// nothing about SQL, HTTP, or Telegram.
package ideas

import "time"

type Stage string

const (
	StageIdea            Stage = "idea"
	StageBriefReady      Stage = "brief_ready"
	StageRecipeReview    Stage = "recipe_review"
	StageScriptReady     Stage = "script_ready"
	StageProductionReady Stage = "production_ready"
)

type Source string

const (
	SourceWeb           Source = "web"
	SourceTelegramText  Source = "telegram_text"
	SourceTelegramVoice Source = "telegram_voice"
)

type EnrichmentStatus string

const (
	EnrichPending EnrichmentStatus = "pending"
	EnrichOK      EnrichmentStatus = "ok"
	EnrichFailed  EnrichmentStatus = "failed"
)

// Metadata is everything Claude infers about an idea. Every field is
// optional: an idea is fully usable before enrichment lands, and must be.
type Metadata struct {
	Title             string   `json:"title"`
	Difficulty        string   `json:"difficulty"`         // easy | moderate | insane
	DurationClass     string   `json:"duration_class"`     // quick | average | multi_day
	Treatment         string   `json:"treatment"`          // elevated | non_elevated
	ContentType       string   `json:"content_type"`       // recipe | vlog
	Cuisine           string   `json:"cuisine"`
	PrimaryIngredient string   `json:"primary_ingredient"`
	Equipment         []string `json:"equipment"`
	VisualPotential   string   `json:"visual_potential"`   // low | medium | high
	Seasonality       string   `json:"seasonality"`        // spring | summer | fall | winter | all_year
	ProductionEffort  string   `json:"production_effort"`  // light | average | heavy
	Tags              []string `json:"tags"`
}

// Enrichment records what happened on the Claude call. Error holds the
// provider's message verbatim so a dead key reads as "401
// authentication_error" on the row instead of as a spinner that never stops.
type Enrichment struct {
	Status     EnrichmentStatus `json:"status"`
	Error      string           `json:"error,omitempty"`
	Model      string           `json:"model,omitempty"`
	EnrichedAt *time.Time       `json:"enriched_at,omitempty"`
}

type Note struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Idea struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	RawText   string  `json:"raw_text"`
	Source    Source  `json:"source"`
	SourceRef string  `json:"source_ref,omitempty"`
	Stage     Stage   `json:"stage"`

	// ArchivedAt is separate from Stage on purpose. Archiving must not
	// destroy how far an idea got through the pipeline.
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	MergedIntoID *string    `json:"merged_into_id,omitempty"`

	Metadata Metadata `json:"metadata"`

	// FieldOverrides names metadata fields a human has corrected.
	// Re-enrichment must not overwrite anything listed here.
	FieldOverrides []string `json:"field_overrides"`

	Enrichment Enrichment `json:"enrichment"`

	Notes     []Note   `json:"notes"`
	LinkedIDs []string `json:"linked_ids"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (i Idea) IsArchived() bool { return i.ArchivedAt != nil }
func (i Idea) IsMerged() bool   { return i.MergedIntoID != nil }

// HasOverride reports whether a human has corrected the named field.
func (i Idea) HasOverride(field string) bool {
	for _, f := range i.FieldOverrides {
		if f == field {
			return true
		}
	}
	return false
}

// ArchivedScope selects which archived state a listing includes.
type ArchivedScope string

const (
	ArchivedExclude ArchivedScope = "false" // default
	ArchivedOnly    ArchivedScope = "true"
	ArchivedAll     ArchivedScope = "all"
)

type ListFilter struct {
	Query      string // when set, FTS rank wins and Sort is ignored
	Stage      string
	Difficulty string
	Duration   string
	Treatment  string
	Archived   ArchivedScope
	Sort       string // created_at | updated_at | title | difficulty | duration
	Order      string // asc | desc
	Limit      int
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/ideas_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func fixtureIdea(id, title string) ideas.Idea {
	now := time.Now().UTC().Truncate(time.Second)
	return ideas.Idea{
		ID:             id,
		Title:          title,
		RawText:        title + " — raw capture text",
		Source:         ideas.SourceWeb,
		Stage:          ideas.StageIdea,
		FieldOverrides: []string{},
		Enrichment:     ideas.Enrichment{Status: ideas.EnrichPending},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestInsertAndGetIdea(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	want := fixtureIdea("i1", "Crispy chili eggs")
	if err := s.InsertIdea(ctx, want); err != nil {
		t.Fatalf("InsertIdea: %v", err)
	}

	got, err := s.GetIdea(ctx, "i1")
	if err != nil {
		t.Fatalf("GetIdea: %v", err)
	}
	if got.Title != want.Title || got.RawText != want.RawText {
		t.Errorf("round trip mismatch: got %+v", got)
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Enrichment.Status = %q, want pending", got.Enrichment.Status)
	}
	if got.Stage != ideas.StageIdea {
		t.Errorf("Stage = %q, want idea", got.Stage)
	}
}

func TestGetIdeaNotFound(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	_, err := s.GetIdea(ctx, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateIdeaPersistsMetadata(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	idea := fixtureIdea("i1", "Sheet-pan shawarma")
	if err := s.InsertIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	enriched := time.Now().UTC().Truncate(time.Second)
	idea.Metadata = ideas.Metadata{
		Difficulty:        "easy",
		DurationClass:     "quick",
		Treatment:         "elevated",
		ContentType:       "recipe",
		Cuisine:           "Middle Eastern",
		PrimaryIngredient: "Chicken",
		Equipment:         []string{"sheet pan", "oven"},
		VisualPotential:   "high",
		Seasonality:       "all_year",
		ProductionEffort:  "light",
	}
	idea.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichOK, Model: "claude-sonnet-5", EnrichedAt: &enriched,
	}
	if err := s.UpdateIdea(ctx, idea); err != nil {
		t.Fatalf("UpdateIdea: %v", err)
	}

	got, err := s.GetIdea(ctx, "i1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Cuisine != "Middle Eastern" {
		t.Errorf("Cuisine = %q", got.Metadata.Cuisine)
	}
	if len(got.Metadata.Equipment) != 2 || got.Metadata.Equipment[0] != "sheet pan" {
		t.Errorf("Equipment = %v, want JSON round trip", got.Metadata.Equipment)
	}
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Enrichment.Status = %q, want ok", got.Enrichment.Status)
	}
}

func TestUpdateIdeaPersistsFailureVerbatim(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	idea := fixtureIdea("i1", "Anything")
	if err := s.InsertIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	const msg = "401 authentication_error: invalid x-api-key (request_id=req_abc)"
	idea.Enrichment = ideas.Enrichment{Status: ideas.EnrichFailed, Error: msg}
	if err := s.UpdateIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetIdea(ctx, "i1")
	if got.Enrichment.Error != msg {
		t.Errorf("error text must survive verbatim.\n got: %q\nwant: %q", got.Enrichment.Error, msg)
	}
}

func TestListExcludesArchivedByDefault(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	active := fixtureIdea("i1", "Active idea")
	archived := fixtureIdea("i2", "Archived idea")
	at := time.Now().UTC()
	archived.ArchivedAt = &at

	for _, i := range []ideas.Idea{active, archived} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListIdeas(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatalf("ListIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Fatalf("default listing should show only active ideas, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedOnly})
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("archived=true should show only archived, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedAll})
	if len(got) != 2 {
		t.Errorf("archived=all should show both, got %d", len(got))
	}
}

func TestListExcludesMergedTombstones(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	primary := fixtureIdea("i1", "Primary")
	dup := fixtureIdea("i2", "Duplicate")
	if err := s.InsertIdea(ctx, primary); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertIdea(ctx, dup); err != nil {
		t.Fatal(err)
	}

	target := "i1"
	dup.MergedIntoID = &target
	if err := s.UpdateIdea(ctx, dup); err != nil {
		t.Fatal(err)
	}

	got, _ := s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedAll})
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("merged tombstones must be excluded from listings, got %d", len(got))
	}

	// but a direct fetch still resolves
	if _, err := s.GetIdea(ctx, "i2"); err != nil {
		t.Errorf("direct fetch of a tombstone should still work: %v", err)
	}
}

func TestListFiltersByMetadata(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	easy := fixtureIdea("i1", "Easy one")
	easy.Metadata.Difficulty = "easy"
	easy.Metadata.DurationClass = "quick"

	hard := fixtureIdea("i2", "Hard one")
	hard.Metadata.Difficulty = "insane"
	hard.Metadata.DurationClass = "multi_day"

	for _, i := range []ideas.Idea{easy, hard} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := s.ListIdeas(ctx, ideas.ListFilter{Difficulty: "easy"})
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("difficulty filter failed, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Duration: "multi_day"})
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("duration filter failed, got %d", len(got))
	}
}

func TestListSortsDifficultySemanticallyNotAlphabetically(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	// Alphabetically: easy, insane, moderate. Semantically: easy, moderate, insane.
	for id, d := range map[string]string{"i1": "insane", "i2": "easy", "i3": "moderate"} {
		idea := fixtureIdea(id, "Idea "+id)
		idea.Metadata.Difficulty = d
		if err := s.InsertIdea(ctx, idea); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListIdeas(ctx, ideas.ListFilter{Sort: "difficulty", Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"easy", "moderate", "insane"}
	for i, w := range want {
		if got[i].Metadata.Difficulty != w {
			t.Errorf("position %d = %q, want %q (semantic order, not alphabetical)",
				i, got[i].Metadata.Difficulty, w)
		}
	}
}

func TestDeleteIdea(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	if err := s.InsertIdea(ctx, fixtureIdea("i1", "Doomed")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteIdea(ctx, "i1"); err != nil {
		t.Fatalf("DeleteIdea: %v", err)
	}
	if _, err := s.GetIdea(ctx, "i1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}
```

- [ ] **Step 3: Run it to make sure it fails**

Run: `go test ./internal/store/ -run TestInsertAndGetIdea -v`
Expected: FAIL — `undefined: (*Store).InsertIdea`

- [ ] **Step 4: Write the implementation**

Create `internal/store/ideas.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

var ErrNotFound = errors.New("not found")

const ideaColumns = `
	id, title, raw_text, source, source_ref, stage, archived_at, merged_into_id,
	difficulty, duration_class, treatment, content_type, cuisine, primary_ingredient,
	equipment, visual_potential, seasonality, production_effort, field_overrides,
	enrichment_status, enrichment_error, enrichment_model, enriched_at,
	created_at, updated_at`

func encodeJSON(v any) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeStrings(raw sql.NullString) []string {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw.String), &out); err != nil {
		return nil
	}
	return out
}

func (s *Store) InsertIdea(ctx context.Context, i ideas.Idea) error {
	equipment, err := encodeJSON(i.Metadata.Equipment)
	if err != nil {
		return fmt.Errorf("encode equipment: %w", err)
	}
	overrides, err := encodeJSON(i.FieldOverrides)
	if err != nil {
		return fmt.Errorf("encode field_overrides: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ideas (`+ideaColumns+`)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?,?,?,?, ?,?,?,?,?, ?,?,?,?, ?,?)`,
		i.ID, i.Title, i.RawText, string(i.Source), nullString(i.SourceRef), string(i.Stage),
		i.ArchivedAt, i.MergedIntoID,
		nullString(i.Metadata.Difficulty), nullString(i.Metadata.DurationClass),
		nullString(i.Metadata.Treatment), nullString(i.Metadata.ContentType),
		nullString(i.Metadata.Cuisine), nullString(i.Metadata.PrimaryIngredient),
		equipment, nullString(i.Metadata.VisualPotential),
		nullString(i.Metadata.Seasonality), nullString(i.Metadata.ProductionEffort),
		overrides,
		string(i.Enrichment.Status), nullString(i.Enrichment.Error),
		nullString(i.Enrichment.Model), i.Enrichment.EnrichedAt,
		i.CreatedAt, i.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert idea %s: %w", i.ID, err)
	}
	return nil
}

func (s *Store) UpdateIdea(ctx context.Context, i ideas.Idea) error {
	equipment, err := encodeJSON(i.Metadata.Equipment)
	if err != nil {
		return fmt.Errorf("encode equipment: %w", err)
	}
	overrides, err := encodeJSON(i.FieldOverrides)
	if err != nil {
		return fmt.Errorf("encode field_overrides: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET
			title = ?, raw_text = ?, source = ?, source_ref = ?, stage = ?,
			archived_at = ?, merged_into_id = ?,
			difficulty = ?, duration_class = ?, treatment = ?, content_type = ?,
			cuisine = ?, primary_ingredient = ?, equipment = ?, visual_potential = ?,
			seasonality = ?, production_effort = ?, field_overrides = ?,
			enrichment_status = ?, enrichment_error = ?, enrichment_model = ?, enriched_at = ?,
			updated_at = ?
		WHERE id = ?`,
		i.Title, i.RawText, string(i.Source), nullString(i.SourceRef), string(i.Stage),
		i.ArchivedAt, i.MergedIntoID,
		nullString(i.Metadata.Difficulty), nullString(i.Metadata.DurationClass),
		nullString(i.Metadata.Treatment), nullString(i.Metadata.ContentType),
		nullString(i.Metadata.Cuisine), nullString(i.Metadata.PrimaryIngredient),
		equipment, nullString(i.Metadata.VisualPotential),
		nullString(i.Metadata.Seasonality), nullString(i.Metadata.ProductionEffort),
		overrides,
		string(i.Enrichment.Status), nullString(i.Enrichment.Error),
		nullString(i.Enrichment.Model), i.Enrichment.EnrichedAt,
		time.Now().UTC(), i.ID,
	)
	if err != nil {
		return fmt.Errorf("update idea %s: %w", i.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetIdea(ctx context.Context, id string) (ideas.Idea, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+ideaColumns+` FROM ideas WHERE id = ?`, id)
	i, err := scanIdea(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ideas.Idea{}, ErrNotFound
	}
	return i, err
}

func (s *Store) DeleteIdea(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ideas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete idea %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// sortExpression maps a sort key to SQL. Difficulty and duration sort by
// their semantic order rather than alphabetically — "insane" must come after
// "moderate", which a plain ORDER BY on the text column would get wrong.
func sortExpression(sort string) string {
	switch sort {
	case "updated_at":
		return "updated_at"
	case "title":
		return "title COLLATE NOCASE"
	case "difficulty":
		return `CASE difficulty WHEN 'easy' THEN 1 WHEN 'moderate' THEN 2
		                        WHEN 'insane' THEN 3 ELSE 4 END`
	case "duration":
		return `CASE duration_class WHEN 'quick' THEN 1 WHEN 'average' THEN 2
		                            WHEN 'multi_day' THEN 3 ELSE 4 END`
	default:
		return "created_at"
	}
}

func defaultOrder(sort string) string {
	if sort == "" || sort == "created_at" || sort == "updated_at" {
		return "DESC"
	}
	return "ASC"
}

func (s *Store) ListIdeas(ctx context.Context, f ideas.ListFilter) ([]ideas.Idea, error) {
	var (
		where = []string{"merged_into_id IS NULL"}
		args  []any
	)

	switch f.Archived {
	case ideas.ArchivedOnly:
		where = append(where, "archived_at IS NOT NULL")
	case ideas.ArchivedAll:
		// no clause
	default:
		where = append(where, "archived_at IS NULL")
	}

	for col, val := range map[string]string{
		"stage":          f.Stage,
		"difficulty":     f.Difficulty,
		"duration_class": f.Duration,
		"treatment":      f.Treatment,
	} {
		if val != "" {
			where = append(where, col+" = ?")
			args = append(args, val)
		}
	}

	order := strings.ToUpper(f.Order)
	if order != "ASC" && order != "DESC" {
		order = defaultOrder(f.Sort)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	query := `SELECT ` + ideaColumns + ` FROM ideas WHERE ` +
		strings.Join(where, " AND ") +
		` ORDER BY ` + sortExpression(f.Sort) + ` ` + order +
		` LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list ideas: %w", err)
	}
	defer rows.Close()

	out := []ideas.Idea{}
	for rows.Next() {
		i, err := scanIdea(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanIdea(sc scanner) (ideas.Idea, error) {
	var (
		i                                    ideas.Idea
		source, stage, enrichStatus          string
		sourceRef, mergedInto                sql.NullString
		difficulty, duration, treatment      sql.NullString
		contentType, cuisine, ingredient     sql.NullString
		equipment, overrides                 sql.NullString
		visual, seasonality, effort          sql.NullString
		enrichErr, enrichModel               sql.NullString
		archivedAt, enrichedAt               sql.NullTime
	)

	err := sc.Scan(
		&i.ID, &i.Title, &i.RawText, &source, &sourceRef, &stage, &archivedAt, &mergedInto,
		&difficulty, &duration, &treatment, &contentType, &cuisine, &ingredient,
		&equipment, &visual, &seasonality, &effort, &overrides,
		&enrichStatus, &enrichErr, &enrichModel, &enrichedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return ideas.Idea{}, err
	}

	i.Source = ideas.Source(source)
	i.Stage = ideas.Stage(stage)
	i.SourceRef = sourceRef.String
	if archivedAt.Valid {
		t := archivedAt.Time
		i.ArchivedAt = &t
	}
	if mergedInto.Valid {
		v := mergedInto.String
		i.MergedIntoID = &v
	}

	i.Metadata = ideas.Metadata{
		Difficulty:        difficulty.String,
		DurationClass:     duration.String,
		Treatment:         treatment.String,
		ContentType:       contentType.String,
		Cuisine:           cuisine.String,
		PrimaryIngredient: ingredient.String,
		Equipment:         decodeStrings(equipment),
		VisualPotential:   visual.String,
		Seasonality:       seasonality.String,
		ProductionEffort:  effort.String,
	}

	i.FieldOverrides = decodeStrings(overrides)
	if i.FieldOverrides == nil {
		i.FieldOverrides = []string{}
	}

	i.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichmentStatus(enrichStatus),
		Error:  enrichErr.String,
		Model:  enrichModel.String,
	}
	if enrichedAt.Valid {
		t := enrichedAt.Time
		i.Enrichment.EnrichedAt = &t
	}

	i.Notes = []ideas.Note{}
	i.LinkedIDs = []string{}
	return i, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Run the tests to make sure they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS — all Task 3 and Task 4 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/ideas internal/store
git commit -m "feat(store): idea persistence with filtering and semantic sort

Difficulty and duration sort through a CASE expression so 'insane' lands
after 'moderate' rather than alphabetically between 'easy' and it.
Listings exclude merged tombstones while direct fetch still resolves them,
so old Telegram references keep working.

Enrichment error text is asserted to survive verbatim — that string is
what turns a dead key into a legible row instead of a silent stall.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: FTS5 search

This is what makes Telegram search return tappable results instead of IDs to copy-paste.

**Files:**
- Create: `internal/store/search.go`, `internal/store/search_test.go`

**Interfaces:**
- Consumes: `store.Store`, `ideas.ListFilter` from Task 4.
- Produces: `(*Store).SearchIdeas(ctx, query string, limit int) ([]ideas.Idea, error)`; `(*Store).ReindexTags(ctx, ideaID string, tags []string) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/store/search_test.go`:

```go
package store

import (
	"context"
	"testing"

	"github.com/erikhoward/souschef/internal/ideas"
)

func seedSearchCorpus(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()

	shawarma := fixtureIdea("i1", "Sheet-pan shawarma")
	shawarma.RawText = "sheet pan shawarma with a lemony feta situation"
	shawarma.Metadata.Cuisine = "Middle Eastern"
	shawarma.Metadata.PrimaryIngredient = "Chicken"

	eggs := fixtureIdea("i2", "Crispy chili eggs")
	eggs.RawText = "chili crisp eggs with scallion oil, very fast"
	eggs.Metadata.Cuisine = "Chinese-inspired"
	eggs.Metadata.PrimaryIngredient = "Eggs"

	soup := fixtureIdea("i3", "Cabbage soup")
	soup.RawText = "humble cabbage soup, slow"

	for _, i := range []ideas.Idea{shawarma, eggs, soup} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
		// Metadata set post-insert must reach the index.
		if err := s.UpdateIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchMatchesTitle(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, err := s.SearchIdeas(context.Background(), "shawarma", 10)
	if err != nil {
		t.Fatalf("SearchIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Fatalf("want just i1, got %d results", len(got))
	}
}

func TestSearchMatchesRawText(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, _ := s.SearchIdeas(context.Background(), "scallion", 10)
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("raw_text should be searchable, got %d results", len(got))
	}
}

func TestSearchMatchesCuisineAndIngredient(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	if got, _ := s.SearchIdeas(context.Background(), "Chinese", 10); len(got) != 1 {
		t.Errorf("cuisine should be searchable, got %d", len(got))
	}
	if got, _ := s.SearchIdeas(context.Background(), "chicken", 10); len(got) != 1 {
		t.Errorf("primary_ingredient should be searchable, got %d", len(got))
	}
}

// Prefix matching is what makes typing a partial word in Telegram useful.
func TestSearchPrefixMatches(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, err := s.SearchIdeas(context.Background(), "shawar", 10)
	if err != nil {
		t.Fatalf("SearchIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("partial word should match via prefix, got %d results", len(got))
	}
}

func TestSearchExcludesArchivedAndMerged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedSearchCorpus(t, s)

	soup, _ := s.GetIdea(ctx, "i3")
	at := soup.CreatedAt
	soup.ArchivedAt = &at
	if err := s.UpdateIdea(ctx, soup); err != nil {
		t.Fatal(err)
	}

	got, _ := s.SearchIdeas(ctx, "cabbage", 10)
	if len(got) != 0 {
		t.Errorf("archived ideas must not appear in search, got %d", len(got))
	}
}

func TestSearchHandlesFTSSyntaxWithoutError(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	// A user typing punctuation must not produce a syntax error from FTS5.
	for _, q := range []string{`"unbalanced`, `AND`, `*`, `foo NEAR/`, `()`} {
		if _, err := s.SearchIdeas(context.Background(), q, 10); err != nil {
			t.Errorf("query %q should be sanitised, not error: %v", q, err)
		}
	}
}

func TestSearchIsRankOrdered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// "eggs" in the title should outrank "eggs" mentioned once in body text.
	strong := fixtureIdea("s1", "Eggs eggs eggs")
	strong.RawText = "eggs"
	weak := fixtureIdea("s2", "Something else")
	weak.RawText = "there are eggs in this one somewhere among many other words"

	for _, i := range []ideas.Idea{strong, weak} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.SearchIdeas(ctx, "eggs", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d", len(got))
	}
	if got[0].ID != "s1" {
		t.Errorf("results must be rank ordered; got %q first", got[0].ID)
	}
}

func TestReindexTagsMakesTagsSearchable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertIdea(ctx, fixtureIdea("i1", "Nondescript")); err != nil {
		t.Fatal(err)
	}
	if err := s.ReindexTags(ctx, "i1", []string{"weeknight", "charred"}); err != nil {
		t.Fatalf("ReindexTags: %v", err)
	}

	got, _ := s.SearchIdeas(ctx, "weeknight", 10)
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("tags should be searchable after reindex, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/store/ -run TestSearch -v`
Expected: FAIL — `undefined: (*Store).SearchIdeas`

- [ ] **Step 3: Write the implementation**

Create `internal/store/search.go`:

```go
package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/erikhoward/souschef/internal/ideas"
)

// sanitiseFTSQuery converts free-typed user input into a safe FTS5 MATCH
// expression. Users type things like `"unbalanced` or `NEAR/` without meaning
// FTS operators, and raw input would raise a syntax error rather than return
// nothing — so we strip everything non-alphanumeric, quote each remaining
// token, and append * for prefix matching.
func sanitiseFTSQuery(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		terms = append(terms, `"`+f+`"*`)
	}
	return strings.Join(terms, " ")
}

// SearchIdeas runs a ranked full-text search. Archived and merged ideas are
// excluded — search is for finding things you might still act on.
func (s *Store) SearchIdeas(ctx context.Context, query string, limit int) ([]ideas.Idea, error) {
	match := sanitiseFTSQuery(query)
	if match == "" {
		return []ideas.Idea{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+prefixed(ideaColumns, "i.")+`
		  FROM ideas_fts f
		  JOIN ideas i ON i.id = f.id
		 WHERE ideas_fts MATCH ?
		   AND i.archived_at IS NULL
		   AND i.merged_into_id IS NULL
		 ORDER BY rank
		 LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	defer rows.Close()

	out := []ideas.Idea{}
	for rows.Next() {
		i, err := scanIdea(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ReindexTags refreshes the denormalised tag text in the FTS index. Tags live
// in a join table, so the AFTER UPDATE trigger on ideas cannot see them.
func (s *Store) ReindexTags(ctx context.Context, ideaID string, tags []string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ideas_fts SET tags = ? WHERE id = ?`,
		strings.Join(tags, " "), ideaID)
	if err != nil {
		return fmt.Errorf("reindex tags for %s: %w", ideaID, err)
	}
	return nil
}

// prefixed qualifies each column in a comma-separated list with a table alias.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for idx, p := range parts {
		parts[idx] = alias + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
```

- [ ] **Step 4: Run the tests to make sure they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS — all store tests.

If `TestSearchIsRankOrdered` fails, confirm the query uses bare `ORDER BY rank` (FTS5's built-in ordering) and not `ORDER BY f.rank`.

- [ ] **Step 5: Commit**

```bash
git add internal/store/search.go internal/store/search_test.go
git commit -m "feat(store): ranked FTS5 search with prefix matching

Free-typed input is sanitised into a safe MATCH expression — a stray
quote or the word AND would otherwise raise an FTS syntax error instead
of returning nothing. Each token gets a * so partial words match, which
is what makes searching from a phone keyboard workable.

This is the fix for the previous iteration's real defect: search had no
index, so finding an idea meant copy-pasting IDs.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Domain service — create, correct, archive, link, merge

This task holds the rules that the spec calls out as non-obvious: archive must
preserve `stage`, corrections must survive re-enrichment, links are symmetric,
and merge leaves a resolvable tombstone.

**Files:**
- Create: `internal/store/relations.go`, `internal/ideas/service.go`, `internal/ideas/service_test.go`

**Interfaces:**
- Consumes: everything from Tasks 4 and 5.
- Produces:
  - `ideas.Repo` interface (what the service needs from `store`)
  - `ideas.NewService(repo Repo) *Service`
  - `(*Service).Create(ctx, rawText string, source Source, sourceRef string) (Idea, error)`
  - `(*Service).ApplyEnrichment(ctx, id string, m Metadata, model string) (Idea, error)`
  - `(*Service).RecordEnrichmentFailure(ctx, id, errText string) (Idea, error)`
  - `(*Service).Correct(ctx, id string, patch map[string]any) (Idea, error)`
  - `(*Service).Archive/Restore/Delete(ctx, id string) (...)`
  - `(*Service).Link(ctx, a, b string) error`, `(*Service).Unlink(ctx, a, b string) error`
  - `(*Service).Merge(ctx, primaryID, duplicateID string) (Idea, error)`
  - `(*Service).AddNote(ctx, id, body string) (Note, error)`
  - `ideas.DeriveTitle(rawText string) string`
  - errors `ideas.ErrSelfLink`, `ideas.ErrSelfMerge`, `ideas.ErrEmptyText`, `ideas.ErrTooLong`

- [ ] **Step 1: Write the relations store**

Create `internal/store/relations.go`:

```go
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func (s *Store) AddNote(ctx context.Context, id, ideaID, body string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (id, idea_id, body, created_at) VALUES (?,?,?,?)`,
		id, ideaID, body, at)
	if err != nil {
		return fmt.Errorf("add note to %s: %w", ideaID, err)
	}
	return nil
}

func (s *Store) DeleteNote(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete note %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) NotesFor(ctx context.Context, ideaID string) ([]ideas.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM notes WHERE idea_id = ? ORDER BY created_at`, ideaID)
	if err != nil {
		return nil, fmt.Errorf("notes for %s: %w", ideaID, err)
	}
	defer rows.Close()

	out := []ideas.Note{}
	for rows.Next() {
		var n ideas.Note
		if err := rows.Scan(&n.ID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// orderPair returns the two ids in canonical order. The idea_links CHECK
// constraint requires a < b, which is how symmetry is enforced by the schema
// rather than by convention.
func orderPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func (s *Store) AddLink(ctx context.Context, a, b string) error {
	lo, hi := orderPair(a, b)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO idea_links (idea_a_id, idea_b_id) VALUES (?,?)`, lo, hi)
	if err != nil {
		return fmt.Errorf("link %s<->%s: %w", a, b, err)
	}
	return nil
}

func (s *Store) RemoveLink(ctx context.Context, a, b string) error {
	lo, hi := orderPair(a, b)
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM idea_links WHERE idea_a_id = ? AND idea_b_id = ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("unlink %s<->%s: %w", a, b, err)
	}
	return nil
}

// LinkedIDs returns every idea linked to id, in either direction.
func (s *Store) LinkedIDs(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT idea_b_id FROM idea_links WHERE idea_a_id = ?
		UNION
		SELECT idea_a_id FROM idea_links WHERE idea_b_id = ?`, id, id)
	if err != nil {
		return nil, fmt.Errorf("linked ids for %s: %w", id, err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return nil, err
		}
		out = append(out, other)
	}
	return out, rows.Err()
}

func (s *Store) SetTags(ctx context.Context, ideaID string, tags []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM idea_tags WHERE idea_id = ?`, ideaID); err != nil {
		return fmt.Errorf("clear tags for %s: %w", ideaID, err)
	}
	for _, name := range tags {
		if name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tags (id, name) VALUES (lower(hex(randomblob(8))), ?)`, name); err != nil {
			return fmt.Errorf("upsert tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO idea_tags (idea_id, tag_id)
			SELECT ?, id FROM tags WHERE name = ?`, ideaID, name); err != nil {
			return fmt.Errorf("attach tag %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.ReindexTags(ctx, ideaID, tags)
}

func (s *Store) TagsFor(ctx context.Context, ideaID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name FROM tags t
		  JOIN idea_tags it ON it.tag_id = t.id
		 WHERE it.idea_id = ? ORDER BY t.name`, ideaID)
	if err != nil {
		return nil, fmt.Errorf("tags for %s: %w", ideaID, err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/ideas/service_test.go`:

```go
package ideas_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

func newService(t *testing.T) (*ideas.Service, context.Context) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return ideas.NewService(s), context.Background()
}

func TestCreateDerivesTitleAndStaysPending(t *testing.T) {
	svc, ctx := newService(t)

	got, err := svc.Create(ctx, "Sheet-pan shawarma with a lemony feta situation. Maybe halloumi too.",
		ideas.SourceWeb, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("Create must assign an id")
	}
	if got.Title != "Sheet-pan shawarma with a lemony feta situation" {
		t.Errorf("Title = %q, want the first sentence", got.Title)
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Status = %q, want pending — capture must never block on Claude", got.Enrichment.Status)
	}
	if got.Stage != ideas.StageIdea {
		t.Errorf("Stage = %q, want idea", got.Stage)
	}
}

func TestDeriveTitleTruncatesLongSingleSentence(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := ideas.DeriveTitle(long)
	if len(got) > 60 {
		t.Errorf("len = %d, want <= 60", len(got))
	}
}

func TestCreateRejectsEmptyAndOverlongText(t *testing.T) {
	svc, ctx := newService(t)

	if _, err := svc.Create(ctx, "   ", ideas.SourceWeb, ""); !errors.Is(err, ideas.ErrEmptyText) {
		t.Errorf("want ErrEmptyText, got %v", err)
	}
	if _, err := svc.Create(ctx, strings.Repeat("x", 5001), ideas.SourceWeb, ""); !errors.Is(err, ideas.ErrTooLong) {
		t.Errorf("want ErrTooLong, got %v", err)
	}
}

// The headline invariant from the spec.
func TestArchiveRestorePreservesStage(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "An idea that got somewhere", ideas.SourceWeb, "")
	created.Stage = ideas.StageBriefReady
	if err := svc.Save(ctx, created); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Archive(ctx, created.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	archived, _ := svc.Get(ctx, created.ID)
	if !archived.IsArchived() {
		t.Fatal("Archive did not set archived_at")
	}
	if archived.Stage != ideas.StageBriefReady {
		t.Errorf("archiving destroyed stage: got %q, want brief_ready", archived.Stage)
	}

	if _, err := svc.Restore(ctx, created.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restored, _ := svc.Get(ctx, created.ID)
	if restored.IsArchived() {
		t.Error("Restore did not clear archived_at")
	}
	if restored.Stage != ideas.StageBriefReady {
		t.Errorf("restore reset stage to %q, want brief_ready preserved", restored.Stage)
	}
}

func TestCorrectRecordsOverrideAndSurvivesReenrichment(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "Chili eggs", ideas.SourceWeb, "")

	// First enrichment lands.
	_, err := svc.ApplyEnrichment(ctx, created.ID, ideas.Metadata{
		Title: "Chili eggs", Difficulty: "moderate", Cuisine: "Unclear",
	}, "claude-sonnet-5")
	if err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	// Human corrects difficulty.
	corrected, err := svc.Correct(ctx, created.ID, map[string]any{"difficulty": "easy"})
	if err != nil {
		t.Fatalf("Correct: %v", err)
	}
	if corrected.Metadata.Difficulty != "easy" {
		t.Errorf("Difficulty = %q, want easy", corrected.Metadata.Difficulty)
	}
	if !corrected.HasOverride("difficulty") {
		t.Fatal("Correct must record the field in FieldOverrides")
	}

	// Re-enrichment tries to set it back and must be refused for that field only.
	after, err := svc.ApplyEnrichment(ctx, created.ID, ideas.Metadata{
		Title: "Chili eggs", Difficulty: "insane", Cuisine: "Chinese-inspired",
	}, "claude-sonnet-5")
	if err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}
	if after.Metadata.Difficulty != "easy" {
		t.Errorf("re-enrichment clobbered a corrected field: got %q, want easy", after.Metadata.Difficulty)
	}
	if after.Metadata.Cuisine != "Chinese-inspired" {
		t.Errorf("re-enrichment should still update uncorrected fields, got %q", after.Metadata.Cuisine)
	}
}

func TestCorrectingTitleProtectsIt(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "some rambling capture", ideas.SourceWeb, "")
	if _, err := svc.Correct(ctx, created.ID, map[string]any{"title": "My Title"}); err != nil {
		t.Fatal(err)
	}

	after, _ := svc.ApplyEnrichment(ctx, created.ID,
		ideas.Metadata{Title: "Model Title", Difficulty: "easy"}, "claude-sonnet-5")
	if after.Title != "My Title" {
		t.Errorf("Title = %q, want the human's title preserved", after.Title)
	}
}

func TestRecordEnrichmentFailureKeepsIdeaUsable(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "Something worth keeping", ideas.SourceWeb, "")
	const msg = "401 authentication_error: invalid x-api-key"

	got, err := svc.RecordEnrichmentFailure(ctx, created.ID, msg)
	if err != nil {
		t.Fatalf("RecordEnrichmentFailure: %v", err)
	}
	if got.Enrichment.Status != ideas.EnrichFailed {
		t.Errorf("Status = %q, want failed", got.Enrichment.Status)
	}
	if got.Enrichment.Error != msg {
		t.Errorf("Error = %q, want verbatim provider message", got.Enrichment.Error)
	}
	if got.RawText != "Something worth keeping" {
		t.Error("the captured text must survive an enrichment failure intact")
	}
}

func TestLinkIsSymmetricAndRejectsSelf(t *testing.T) {
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, "Idea A", ideas.SourceWeb, "")
	b, _ := svc.Create(ctx, "Idea B", ideas.SourceWeb, "")

	if err := svc.Link(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("Link: %v", err)
	}

	gotA, _ := svc.Get(ctx, a.ID)
	gotB, _ := svc.Get(ctx, b.ID)
	if len(gotA.LinkedIDs) != 1 || gotA.LinkedIDs[0] != b.ID {
		t.Errorf("A should link to B, got %v", gotA.LinkedIDs)
	}
	if len(gotB.LinkedIDs) != 1 || gotB.LinkedIDs[0] != a.ID {
		t.Errorf("link must be symmetric; B sees %v", gotB.LinkedIDs)
	}

	// Linking again in the other direction must not create a duplicate.
	if err := svc.Link(ctx, b.ID, a.ID); err != nil {
		t.Fatalf("re-link: %v", err)
	}
	gotA, _ = svc.Get(ctx, a.ID)
	if len(gotA.LinkedIDs) != 1 {
		t.Errorf("reverse link created a duplicate: %v", gotA.LinkedIDs)
	}

	if err := svc.Link(ctx, a.ID, a.ID); !errors.Is(err, ideas.ErrSelfLink) {
		t.Errorf("want ErrSelfLink, got %v", err)
	}
}

func TestMergeUnionsAndTombstones(t *testing.T) {
	svc, ctx := newService(t)

	primary, _ := svc.Create(ctx, "Primary idea", ideas.SourceWeb, "")
	dup, _ := svc.Create(ctx, "Duplicate idea", ideas.SourceWeb, "")
	other, _ := svc.Create(ctx, "Unrelated", ideas.SourceWeb, "")

	if _, err := svc.AddNote(ctx, primary.ID, "note on primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(ctx, dup.ID, "note on duplicate"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, dup.ID, other.ID); err != nil {
		t.Fatal(err)
	}

	merged, err := svc.Merge(ctx, primary.ID, dup.ID)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(merged.Notes) != 2 {
		t.Errorf("merge should union notes, got %d", len(merged.Notes))
	}
	if len(merged.LinkedIDs) != 1 || merged.LinkedIDs[0] != other.ID {
		t.Errorf("merge should inherit the duplicate's links, got %v", merged.LinkedIDs)
	}

	tomb, err := svc.Get(ctx, dup.ID)
	if err != nil {
		t.Fatalf("tombstone must still resolve: %v", err)
	}
	if tomb.MergedIntoID == nil || *tomb.MergedIntoID != primary.ID {
		t.Error("duplicate must be tombstoned pointing at the primary")
	}

	if err := svc.Merge(ctx, primary.ID, primary.ID); err == nil {
		t.Error("merging an idea into itself must fail")
	}
}
```

- [ ] **Step 3: Run it to make sure it fails**

Run: `go test ./internal/ideas/ -v`
Expected: FAIL — `undefined: ideas.NewService`

- [ ] **Step 4: Write the service**

Create `internal/ideas/service.go`:

```go
package ideas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEmptyText = errors.New("idea text must not be empty")
	ErrTooLong   = errors.New("idea text must be 5000 characters or fewer")
	ErrSelfLink  = errors.New("an idea cannot be linked to itself")
	ErrSelfMerge = errors.New("an idea cannot be merged into itself")
)

const maxRawTextLen = 5000

// Repo is the slice of the store the service needs. Declaring it here rather
// than importing a concrete type keeps the dependency pointing inward and lets
// tests substitute a fake if one is ever warranted.
type Repo interface {
	InsertIdea(ctx context.Context, i Idea) error
	GetIdea(ctx context.Context, id string) (Idea, error)
	UpdateIdea(ctx context.Context, i Idea) error
	DeleteIdea(ctx context.Context, id string) error
	ListIdeas(ctx context.Context, f ListFilter) ([]Idea, error)
	SearchIdeas(ctx context.Context, q string, limit int) ([]Idea, error)

	AddNote(ctx context.Context, id, ideaID, body string, at time.Time) error
	NotesFor(ctx context.Context, ideaID string) ([]Note, error)
	AddLink(ctx context.Context, a, b string) error
	RemoveLink(ctx context.Context, a, b string) error
	LinkedIDs(ctx context.Context, id string) ([]string, error)
	SetTags(ctx context.Context, ideaID string, tags []string) error
	TagsFor(ctx context.Context, ideaID string) ([]string, error)
}

type Service struct{ repo Repo }

func NewService(repo Repo) *Service { return &Service{repo: repo} }

// DeriveTitle produces a provisional title from raw capture text so a row is
// never blank before enrichment lands. It prefers the first sentence, and
// falls back to a 60-character truncation on a word boundary.
func DeriveTitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "Untitled idea"
	}
	if idx := strings.IndexAny(s, ".!?\n"); idx > 0 && idx <= 60 {
		return strings.TrimSpace(s[:idx])
	}
	if len(s) <= 60 {
		return s
	}
	cut := s[:60]
	if sp := strings.LastIndex(cut, " "); sp > 20 {
		cut = cut[:sp]
	}
	return strings.TrimSpace(cut) + "…"
}

func (s *Service) Create(ctx context.Context, rawText string, source Source, sourceRef string) (Idea, error) {
	text := strings.TrimSpace(rawText)
	if text == "" {
		return Idea{}, ErrEmptyText
	}
	if len(text) > maxRawTextLen {
		return Idea{}, ErrTooLong
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Idea{}, fmt.Errorf("generate id: %w", err)
	}

	now := time.Now().UTC()
	idea := Idea{
		ID:             id.String(),
		Title:          DeriveTitle(text),
		RawText:        text,
		Source:         source,
		SourceRef:      sourceRef,
		Stage:          StageIdea,
		FieldOverrides: []string{},
		Enrichment:     Enrichment{Status: EnrichPending},
		Notes:          []Note{},
		LinkedIDs:      []string{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.InsertIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return idea, nil
}

// Get returns an idea with its notes and links populated.
func (s *Service) Get(ctx context.Context, id string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

func (s *Service) hydrate(ctx context.Context, idea Idea) (Idea, error) {
	notes, err := s.repo.NotesFor(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	linked, err := s.repo.LinkedIDs(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	tags, err := s.repo.TagsFor(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	idea.Notes = notes
	idea.LinkedIDs = linked
	idea.Metadata.Tags = tags
	return idea, nil
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Idea, error) {
	if f.Query != "" {
		return s.repo.SearchIdeas(ctx, f.Query, f.Limit)
	}
	return s.repo.ListIdeas(ctx, f)
}

func (s *Service) Save(ctx context.Context, i Idea) error {
	return s.repo.UpdateIdea(ctx, i)
}

// ApplyEnrichment writes inferred metadata, skipping every field a human has
// already corrected. This is the invariant that lets "allow correction" and
// "retry enrichment" coexist.
func (s *Service) ApplyEnrichment(ctx context.Context, id string, m Metadata, model string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}

	set := func(field string, dst *string, val string) {
		if val != "" && !idea.HasOverride(field) {
			*dst = val
		}
	}

	if m.Title != "" && !idea.HasOverride("title") {
		idea.Title = m.Title
	}
	set("difficulty", &idea.Metadata.Difficulty, m.Difficulty)
	set("duration_class", &idea.Metadata.DurationClass, m.DurationClass)
	set("treatment", &idea.Metadata.Treatment, m.Treatment)
	set("content_type", &idea.Metadata.ContentType, m.ContentType)
	set("cuisine", &idea.Metadata.Cuisine, m.Cuisine)
	set("primary_ingredient", &idea.Metadata.PrimaryIngredient, m.PrimaryIngredient)
	set("visual_potential", &idea.Metadata.VisualPotential, m.VisualPotential)
	set("seasonality", &idea.Metadata.Seasonality, m.Seasonality)
	set("production_effort", &idea.Metadata.ProductionEffort, m.ProductionEffort)

	if len(m.Equipment) > 0 && !idea.HasOverride("equipment") {
		idea.Metadata.Equipment = m.Equipment
	}

	now := time.Now().UTC()
	idea.Enrichment = Enrichment{Status: EnrichOK, Model: model, EnrichedAt: &now}
	idea.UpdatedAt = now

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	if len(m.Tags) > 0 && !idea.HasOverride("tags") {
		if err := s.repo.SetTags(ctx, idea.ID, m.Tags); err != nil {
			return Idea{}, err
		}
	}
	return s.hydrate(ctx, idea)
}

// RecordEnrichmentFailure stores the provider's message verbatim. The idea
// itself is untouched and remains fully usable.
func (s *Service) RecordEnrichmentFailure(ctx context.Context, id, errText string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	idea.Enrichment.Status = EnrichFailed
	idea.Enrichment.Error = errText
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

// MarkPending resets an idea for a retry.
func (s *Service) MarkPending(ctx context.Context, id string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	idea.Enrichment = Enrichment{Status: EnrichPending}
	idea.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

// correctable maps a JSON patch key to the field it writes.
var correctable = map[string]func(*Idea, string){
	"title":              func(i *Idea, v string) { i.Title = v },
	"difficulty":         func(i *Idea, v string) { i.Metadata.Difficulty = v },
	"duration_class":     func(i *Idea, v string) { i.Metadata.DurationClass = v },
	"treatment":          func(i *Idea, v string) { i.Metadata.Treatment = v },
	"content_type":       func(i *Idea, v string) { i.Metadata.ContentType = v },
	"cuisine":            func(i *Idea, v string) { i.Metadata.Cuisine = v },
	"primary_ingredient": func(i *Idea, v string) { i.Metadata.PrimaryIngredient = v },
	"visual_potential":   func(i *Idea, v string) { i.Metadata.VisualPotential = v },
	"seasonality":        func(i *Idea, v string) { i.Metadata.Seasonality = v },
	"production_effort":  func(i *Idea, v string) { i.Metadata.ProductionEffort = v },
}

// Correct applies a human edit and records the field as overridden so
// re-enrichment leaves it alone.
func (s *Service) Correct(ctx context.Context, id string, patch map[string]any) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}

	overrides := map[string]bool{}
	for _, f := range idea.FieldOverrides {
		overrides[f] = true
	}

	for key, raw := range patch {
		switch key {
		case "equipment", "tags":
			vals, ok := toStringSlice(raw)
			if !ok {
				return Idea{}, fmt.Errorf("%s must be an array of strings", key)
			}
			if key == "equipment" {
				idea.Metadata.Equipment = vals
			} else {
				if err := s.repo.SetTags(ctx, idea.ID, vals); err != nil {
					return Idea{}, err
				}
			}
			overrides[key] = true
		default:
			apply, known := correctable[key]
			if !known {
				return Idea{}, fmt.Errorf("field %q is not correctable", key)
			}
			str, ok := raw.(string)
			if !ok {
				return Idea{}, fmt.Errorf("%s must be a string", key)
			}
			apply(&idea, str)
			overrides[key] = true
		}
	}

	idea.FieldOverrides = idea.FieldOverrides[:0]
	for f := range overrides {
		idea.FieldOverrides = append(idea.FieldOverrides, f)
	}
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

func toStringSlice(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// Archive sets archived_at and deliberately leaves Stage alone.
func (s *Service) Archive(ctx context.Context, id string) (Idea, error) {
	return s.setArchived(ctx, id, true)
}

func (s *Service) Restore(ctx context.Context, id string) (Idea, error) {
	return s.setArchived(ctx, id, false)
}

func (s *Service) setArchived(ctx context.Context, id string, archived bool) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	if archived {
		now := time.Now().UTC()
		idea.ArchivedAt = &now
	} else {
		idea.ArchivedAt = nil
	}
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteIdea(ctx, id)
}

func (s *Service) Link(ctx context.Context, a, b string) error {
	if a == b {
		return ErrSelfLink
	}
	return s.repo.AddLink(ctx, a, b)
}

func (s *Service) Unlink(ctx context.Context, a, b string) error {
	return s.repo.RemoveLink(ctx, a, b)
}

func (s *Service) AddNote(ctx context.Context, ideaID, body string) (Note, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Note{}, ErrEmptyText
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Note{}, fmt.Errorf("generate id: %w", err)
	}
	now := time.Now().UTC()
	if err := s.repo.AddNote(ctx, id.String(), ideaID, body, now); err != nil {
		return Note{}, err
	}
	return Note{ID: id.String(), Body: body, CreatedAt: now}, nil
}

// Merge folds duplicate into primary: notes and links move across, the
// duplicate becomes a tombstone pointing at the primary rather than being
// deleted, so old references still resolve.
func (s *Service) Merge(ctx context.Context, primaryID, duplicateID string) (Idea, error) {
	if primaryID == duplicateID {
		return Idea{}, ErrSelfMerge
	}

	primary, err := s.repo.GetIdea(ctx, primaryID)
	if err != nil {
		return Idea{}, err
	}
	duplicate, err := s.repo.GetIdea(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}

	dupNotes, err := s.repo.NotesFor(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	for _, n := range dupNotes {
		if err := s.repo.AddNote(ctx, uuid.NewString(), primaryID, n.Body, n.CreatedAt); err != nil {
			return Idea{}, err
		}
	}

	dupLinks, err := s.repo.LinkedIDs(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	for _, other := range dupLinks {
		if other == primaryID {
			continue
		}
		if err := s.repo.AddLink(ctx, primaryID, other); err != nil {
			return Idea{}, err
		}
		if err := s.repo.RemoveLink(ctx, duplicateID, other); err != nil {
			return Idea{}, err
		}
	}

	dupTags, err := s.repo.TagsFor(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	if len(dupTags) > 0 {
		primaryTags, err := s.repo.TagsFor(ctx, primaryID)
		if err != nil {
			return Idea{}, err
		}
		seen := map[string]bool{}
		union := []string{}
		for _, t := range append(primaryTags, dupTags...) {
			if !seen[t] {
				seen[t] = true
				union = append(union, t)
			}
		}
		if err := s.repo.SetTags(ctx, primaryID, union); err != nil {
			return Idea{}, err
		}
	}

	duplicate.MergedIntoID = &primaryID
	duplicate.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, duplicate); err != nil {
		return Idea{}, err
	}

	primary.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, primary); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, primary)
}
```

- [ ] **Step 5: Run the tests to make sure they pass**

Run: `go test ./internal/ideas/ ./internal/store/ -v`
Expected: PASS

- [ ] **Step 6: Run the whole suite with the race detector**

Run: `go test ./... -race`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ideas internal/store/relations.go
git commit -m "feat(ideas): domain service with correction, archive, link, merge

Four invariants have explicit tests because they are where this model
earns its keep:
  - archive/restore preserves stage (the old build destroyed it)
  - a corrected field survives re-enrichment; uncorrected ones update
  - links are symmetric and deduplicated via canonical ordering
  - merge tombstones the duplicate so old references still resolve

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Claude enrichment

The package is split into a pure core and a thin I/O shell. `ParseResponse`
and `Classify` are ordinary functions over bytes and errors, so the taxonomy
and the failure handling stay covered by tests that run offline with no API
key — the exact condition that made the previous incident hard to diagnose.

**Files:**
- Create: `internal/enrich/prompt.go`, `internal/enrich/enrich.go`, `internal/enrich/enrich_test.go`
- Create: `internal/enrich/testdata/valid.json`, `internal/enrich/testdata/partial.json`, `internal/enrich/testdata/bad_enum.json`, `internal/enrich/testdata/malformed.json`

**Interfaces:**
- Consumes: `ideas.Metadata` from Task 4; `config.Config.Model` and `.Effort` from Task 2.
- Produces:
  - `enrich.New(model, effort string) *Enricher`
  - `(*Enricher).Enrich(ctx, rawText string) (ideas.Metadata, error)`
  - `enrich.ParseResponse(b []byte) (ideas.Metadata, error)` — pure
  - `enrich.Classify(err error) Failure` — pure
  - `enrich.Failure{Message string; Retryable bool}`
  - `enrich.SystemPrompt` string constant
  - `enrich.MetadataSchema` map — the JSON schema sent as `output_config.format`

- [ ] **Step 1: Add the SDK**

```bash
go get github.com/anthropics/anthropic-sdk-go@latest
```

- [ ] **Step 2: Write the fixtures**

Create `internal/enrich/testdata/valid.json`:

```json
{
  "title": "Crispy chili eggs with scallion oil",
  "difficulty": "easy",
  "duration_class": "quick",
  "treatment": "elevated",
  "content_type": "recipe",
  "cuisine": "Chinese-inspired",
  "primary_ingredient": "Eggs",
  "equipment": ["wok", "slotted spoon"],
  "visual_potential": "high",
  "seasonality": "all_year",
  "production_effort": "light",
  "tags": ["weeknight", "fast", "crispy"]
}
```

Create `internal/enrich/testdata/partial.json` — a valid response that omits optional fields:

```json
{
  "title": "Something vague I mumbled",
  "difficulty": "moderate",
  "duration_class": "average",
  "treatment": "non_elevated",
  "content_type": "recipe",
  "cuisine": "",
  "primary_ingredient": "",
  "equipment": [],
  "visual_potential": "medium",
  "seasonality": "all_year",
  "production_effort": "average",
  "tags": []
}
```

Create `internal/enrich/testdata/bad_enum.json` — schema-shaped but with a value outside the taxonomy:

```json
{
  "title": "Wrong enum",
  "difficulty": "trivial",
  "duration_class": "quick",
  "treatment": "elevated",
  "content_type": "recipe",
  "cuisine": "Italian",
  "primary_ingredient": "Pasta",
  "equipment": [],
  "visual_potential": "high",
  "seasonality": "all_year",
  "production_effort": "light",
  "tags": []
}
```

Create `internal/enrich/testdata/malformed.json`:

```
{"title": "unterminated
```

- [ ] **Step 3: Write the failing test**

Create `internal/enrich/enrich_test.go`:

```go
package enrich

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestParseResponseValid(t *testing.T) {
	got, err := ParseResponse(fixture(t, "valid.json"))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if got.Title != "Crispy chili eggs with scallion oil" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Difficulty != "easy" || got.DurationClass != "quick" {
		t.Errorf("difficulty/duration = %q/%q", got.Difficulty, got.DurationClass)
	}
	if len(got.Equipment) != 2 || got.Equipment[0] != "wok" {
		t.Errorf("Equipment = %v", got.Equipment)
	}
	if len(got.Tags) != 3 {
		t.Errorf("Tags = %v, want 3", got.Tags)
	}
}

func TestParseResponseAcceptsEmptyOptionalFields(t *testing.T) {
	got, err := ParseResponse(fixture(t, "partial.json"))
	if err != nil {
		t.Fatalf("a response with empty optional fields must parse: %v", err)
	}
	if got.Cuisine != "" || got.PrimaryIngredient != "" {
		t.Error("empty strings should stay empty, not be defaulted")
	}
	if got.Difficulty != "moderate" {
		t.Errorf("Difficulty = %q", got.Difficulty)
	}
}

// A model can return well-formed JSON containing a value outside our
// taxonomy. Storing "trivial" as a difficulty would silently corrupt every
// filter and sort downstream, so it must be rejected here.
func TestParseResponseRejectsValueOutsideTaxonomy(t *testing.T) {
	_, err := ParseResponse(fixture(t, "bad_enum.json"))
	if err == nil {
		t.Fatal("expected rejection of difficulty=trivial")
	}
	if !strings.Contains(err.Error(), "difficulty") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

func TestParseResponseRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseResponse(fixture(t, "malformed.json")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseResponseRejectsEmptyTitle(t *testing.T) {
	_, err := ParseResponse([]byte(`{"title":"","difficulty":"easy","duration_class":"quick",
		"treatment":"elevated","content_type":"recipe","visual_potential":"high",
		"seasonality":"all_year","production_effort":"light"}`))
	if err == nil {
		t.Fatal("title is the one field we require; empty must be rejected")
	}
}

func TestClassifyAuthFailureIsNotRetryable(t *testing.T) {
	got := Classify(&stubAPIError{status: 401, msg: "invalid x-api-key", requestID: "req_abc"})

	if got.Retryable {
		t.Error("a 401 must not be retried — the key will not fix itself")
	}
	for _, want := range []string{"401", "invalid x-api-key", "req_abc"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message %q should contain %q", got.Message, want)
		}
	}
}

func TestClassifyRateLimitIsRetryable(t *testing.T) {
	got := Classify(&stubAPIError{status: 429, msg: "rate_limit_error"})
	if !got.Retryable {
		t.Error("429 should be retryable")
	}
	if !strings.Contains(got.Message, "429") {
		t.Errorf("message must carry the status code, got %q", got.Message)
	}
}

func TestClassifyServerErrorIsRetryable(t *testing.T) {
	if !Classify(&stubAPIError{status: 529, msg: "overloaded_error"}).Retryable {
		t.Error("529 should be retryable")
	}
	if !Classify(&stubAPIError{status: 500, msg: "api_error"}).Retryable {
		t.Error("500 should be retryable")
	}
}

func TestClassifyNonAPIErrorStillProducesMessage(t *testing.T) {
	got := Classify(errors.New("dial tcp: lookup api.anthropic.com: no such host"))
	if got.Message == "" {
		t.Fatal("a transport error must still produce a message for the row")
	}
	if !got.Retryable {
		t.Error("transport errors should be retryable")
	}
}

func TestClassifyNilIsNotAFailure(t *testing.T) {
	if got := Classify(nil); got.Message != "" {
		t.Errorf("Classify(nil) should be zero, got %+v", got)
	}
}

func TestSystemPromptCoversEveryTaxonomyValue(t *testing.T) {
	// If a value exists in the schema, the prompt must define it — otherwise
	// the model is guessing at what we mean.
	for _, v := range []string{
		"easy", "moderate", "insane",
		"quick", "average", "multi_day",
		"elevated", "non_elevated",
		"recipe", "vlog",
		"light", "heavy",
	} {
		if !strings.Contains(SystemPrompt, v) {
			t.Errorf("SystemPrompt does not mention taxonomy value %q", v)
		}
	}
}
```

Add the stub error to the same file:

```go
// stubAPIError mimics the shape Classify inspects. The real SDK error type is
// substituted in Step 5 once the exact field names are confirmed against the
// compiler; this keeps the pure logic testable either way.
type stubAPIError struct {
	status    int
	msg       string
	requestID string
}

func (e *stubAPIError) Error() string  { return e.msg }
func (e *stubAPIError) StatusCode() int { return e.status }
func (e *stubAPIError) RequestID() string { return e.requestID }
```

- [ ] **Step 4: Run it to make sure it fails**

Run: `go test ./internal/enrich/ -v`
Expected: FAIL — `undefined: ParseResponse`

- [ ] **Step 5: Write the taxonomy prompt**

Create `internal/enrich/prompt.go`:

```go
package enrich

// SystemPrompt defines the taxonomy. It is byte-identical on every call, which
// makes it the correct cache_control target.
//
// Note the 1024-token minimum cacheable prefix on Sonnet 5: below it, caching
// silently does nothing and reports cache_creation_input_tokens: 0 with no
// error. Verify usage.cache_read_input_tokens against a real call rather than
// assuming the marker took effect.
const SystemPrompt = `You classify recipe and video ideas for a food content creator.

You will be given a raw, unpolished capture — often dictated, often a fragment.
Infer structured metadata from it. Never invent detail the text does not
support: when a field is genuinely unknowable from the input, return an empty
string for it rather than guessing.

TAXONOMY

difficulty — how hard the technique is, not how long it takes:
  easy      Routine technique. Nothing that can go badly wrong.
  moderate  Requires attention or timing. A distracted cook could ruin it.
  insane    Specialist technique, long chains of dependent steps, or a high
            failure rate even when done carefully.

duration_class — wall-clock from starting to eating:
  quick      Under 30 minutes.
  average    30 minutes to about 3 hours.
  multi_day  Requires overnight resting, fermenting, curing, or brining.

treatment:
  elevated      A restaurant-leaning take: refined technique, plating, or an
                unexpected ingredient pairing.
  non_elevated  Straightforward home cooking, presented plainly.

content_type:
  recipe  Produces a dish with a repeatable method.
  vlog    Process, day-in-the-life, or commentary with no reproducible recipe.

visual_potential — how well it will film:
  high    Strong visual moments: sizzle, char, pull, pour, melt, steam.
  medium  Looks good but undramatic.
  low     Tastes better than it looks.

production_effort — the burden on the creator, not the cook:
  light    One setup, minimal prep, few shots.
  average  Some prep staging and a couple of camera setups.
  heavy    Multiple setups, long shoots, or significant cleanup.

seasonality: spring, summer, fall, winter, or all_year when it is not
seasonal.

cuisine: a short label such as "Middle Eastern" or "Chinese-inspired". Prefer
"-inspired" when the dish borrows a technique without claiming authenticity.
Empty string if the text gives no signal.

primary_ingredient: the single ingredient the dish is about, capitalised.
Empty string if unclear.

equipment: specific equipment the method requires. Omit ordinary items such as
a knife, bowl, or stovetop. Empty array when nothing notable is needed.

tags: 2 to 5 short lowercase keywords for later retrieval. Prefer words the
creator would actually search for.

title: a clean, specific title in the creator's voice — dry, competent,
quietly enthusiastic. Never sarcastic, never exclamatory, no generational
references, no wordplay for its own sake. Under 60 characters.`
```

- [ ] **Step 6: Write the implementation**

Create `internal/enrich/enrich.go`:

```go
// Package enrich turns raw capture text into structured metadata using Claude.
//
// It never touches the database. Enrich is a function from text to metadata,
// which makes the expensive, nondeterministic, key-requiring part of the
// system replayable from fixtures in tests.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/erikhoward/souschef/internal/ideas"
)

// taxonomy lists the permitted values per enum field. It is the single source
// of truth for both the JSON schema we send and the validation we apply on
// the way back — a model can return well-formed JSON with a value we never
// asked for, and storing it would corrupt every filter downstream.
var taxonomy = map[string][]string{
	"difficulty":        {"easy", "moderate", "insane"},
	"duration_class":    {"quick", "average", "multi_day"},
	"treatment":         {"elevated", "non_elevated"},
	"content_type":      {"recipe", "vlog"},
	"visual_potential":  {"low", "medium", "high"},
	"seasonality":       {"spring", "summer", "fall", "winter", "all_year"},
	"production_effort": {"light", "average", "heavy"},
}

func enumProp(field string) map[string]any {
	return map[string]any{"type": "string", "enum": taxonomy[field]}
}

// MetadataSchema is sent as output_config.format. Note the deprecated
// top-level output_format parameter is not used anywhere.
var MetadataSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":              map[string]any{"type": "string"},
		"difficulty":         enumProp("difficulty"),
		"duration_class":     enumProp("duration_class"),
		"treatment":          enumProp("treatment"),
		"content_type":       enumProp("content_type"),
		"cuisine":            map[string]any{"type": "string"},
		"primary_ingredient": map[string]any{"type": "string"},
		"equipment":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"visual_potential":   enumProp("visual_potential"),
		"seasonality":        enumProp("seasonality"),
		"production_effort":  enumProp("production_effort"),
		"tags":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{
		"title", "difficulty", "duration_class", "treatment", "content_type",
		"visual_potential", "seasonality", "production_effort",
	},
	"additionalProperties": false,
}

func allowed(field, value string) bool {
	for _, v := range taxonomy[field] {
		if v == value {
			return true
		}
	}
	return false
}

// ParseResponse decodes and validates a model response. It is pure — every
// taxonomy test runs against recorded fixtures with no network and no key.
func ParseResponse(b []byte) (ideas.Metadata, error) {
	var m ideas.Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return ideas.Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}

	if strings.TrimSpace(m.Title) == "" {
		return ideas.Metadata{}, errors.New("metadata: title must not be empty")
	}

	for field, value := range map[string]string{
		"difficulty":        m.Difficulty,
		"duration_class":    m.DurationClass,
		"treatment":         m.Treatment,
		"content_type":      m.ContentType,
		"visual_potential":  m.VisualPotential,
		"seasonality":       m.Seasonality,
		"production_effort": m.ProductionEffort,
	} {
		if !allowed(field, value) {
			return ideas.Metadata{}, fmt.Errorf(
				"metadata: %s = %q is outside the taxonomy (allowed: %s)",
				field, value, strings.Join(taxonomy[field], ", "))
		}
	}

	if m.Equipment == nil {
		m.Equipment = []string{}
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	return m, nil
}

// Failure is a classified enrichment error, ready to be written to the row.
type Failure struct {
	Message   string
	Retryable bool
}

// statusCarrier is satisfied by the SDK's error type and by the test stub.
type statusCarrier interface {
	Error() string
	StatusCode() int
}

type requestIDCarrier interface{ RequestID() string }

// Classify turns any error into a message for the idea row and a retry
// decision. Nothing is swallowed: a transport failure with no status still
// produces text a human can act on.
func Classify(err error) Failure {
	if err == nil {
		return Failure{}
	}

	var sc statusCarrier
	if errors.As(err, &sc) {
		msg := fmt.Sprintf("%d %s", sc.StatusCode(), sc.Error())

		var rc requestIDCarrier
		if errors.As(err, &rc) && rc.RequestID() != "" {
			msg += fmt.Sprintf(" (request_id=%s)", rc.RequestID())
		}

		switch code := sc.StatusCode(); {
		case code == 401 || code == 403:
			// The key is wrong or unauthorized. Retrying cannot help, and
			// retrying quietly is how this became invisible last time.
			return Failure{Message: msg, Retryable: false}
		case code == 400 || code == 404 || code == 413:
			return Failure{Message: msg, Retryable: false}
		case code == 429 || code >= 500:
			return Failure{Message: msg, Retryable: true}
		default:
			return Failure{Message: msg, Retryable: false}
		}
	}

	return Failure{Message: err.Error(), Retryable: true}
}

type Enricher struct {
	client anthropic.Client
	model  string
	effort string
}

// New builds an Enricher. The client resolves credentials itself: an
// ANTHROPIC_API_KEY, an ANTHROPIC_AUTH_TOKEN, or an `ant auth login` profile.
func New(model, effort string) *Enricher {
	return &Enricher{client: anthropic.NewClient(), model: model, effort: effort}
}

const (
	enrichTimeout = 60 * time.Second
	maxAttempts   = 3
)

// Enrich classifies rawText. On a retryable failure it backs off and tries
// again up to maxAttempts; on a terminal failure it returns immediately.
func (e *Enricher) Enrich(ctx context.Context, rawText string) (ideas.Metadata, error) {
	var last error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, enrichTimeout)
		raw, err := e.call(attemptCtx, rawText)
		cancel()

		if err == nil {
			return ParseResponse(raw)
		}
		last = err

		if f := Classify(err); !f.Retryable || attempt == maxAttempts {
			return ideas.Metadata{}, err
		}

		select {
		case <-ctx.Done():
			return ideas.Metadata{}, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return ideas.Metadata{}, last
}
```

- [ ] **Step 7: Write the SDK call, letting the compiler settle the type names**

Append `call` to `internal/enrich/enrich.go`. **Do not research the SDK's exact
type names first — write this, run `go build ./internal/enrich/`, and fix what
the compiler reports.** That loop is faster than reading the repo, and the
field names below are close enough to guide it.

```go
// call issues one request and returns the raw JSON body of the response.
func (e *Enricher) call(ctx context.Context, rawText string) ([]byte, error) {
	msg, err := e.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(e.model),
		MaxTokens: 2048,

		// The taxonomy prompt is byte-identical every call, so it is the
		// cache target. Effort is low: this is a short, scoped classification.
		System: []anthropic.TextBlockParam{{
			Text:         SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},

		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},

		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.Effort(e.effort),
			Format: anthropic.JSONOutputFormatParam{Schema: MetadataSchema},
		},

		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(rawText)),
		},
	})
	if err != nil {
		return nil, err
	}

	// A refusal arrives as HTTP 200 with an empty content array, so this must
	// be checked before indexing into Content.
	if msg.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("model declined to classify this idea")
	}

	for _, block := range msg.Content {
		if block.Type == "text" {
			return []byte(block.Text), nil
		}
	}
	return nil, errors.New("response contained no text block")
}
```

- [ ] **Step 8: Build and fix compiler errors**

Run: `go build ./internal/enrich/`

Expected: some type-name mismatches on the first pass. Fix each against the
compiler's suggestion. Do **not** change the semantics while doing so — in
particular keep adaptive thinking, keep `output_config.format`, and never add
`temperature`, `top_p`, `top_k`, or `budget_tokens`. All four return 400 on
Sonnet 5.

- [ ] **Step 9: Run the tests to make sure they pass**

Run: `go test ./internal/enrich/ -v`
Expected: PASS — all eleven tests, with no API key set.

- [ ] **Step 10: Record a real fixture and verify caching**

This step needs a working credential. Write a throwaway `main` under
`/tmp`, or add a temporary test guarded by an env var:

```go
func TestLiveEnrichment(t *testing.T) {
	if os.Getenv("SOUSCHEF_LIVE_TEST") == "" {
		t.Skip("set SOUSCHEF_LIVE_TEST=1 to exercise the real API")
	}
	e := New("claude-sonnet-5", "low")
	got, err := e.Enrich(context.Background(),
		"sheet pan shawarma with a lemony feta situation, weeknight thing")
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	t.Logf("%+v", got)
	if got.Title == "" {
		t.Error("live call returned an empty title")
	}
}
```

Run: `SOUSCHEF_LIVE_TEST=1 go test ./internal/enrich/ -run TestLiveEnrichment -v`

Then **check the cache actually engaged.** Log `msg.Usage` from `call` and
confirm `CacheReadInputTokens` is non-zero on the *second* consecutive run. If
it is zero on both runs, the taxonomy prompt is under Sonnet 5's 1024-token
minimum and `cache_control` is silently doing nothing — either accept that
(correctness is unaffected) or lengthen the prompt with genuinely useful
guidance. Record the finding in `PROJECT_LOG.md` either way.

- [ ] **Step 11: Commit**

```bash
git add internal/enrich
git commit -m "feat(enrich): Claude metadata inference with offline-testable core

ParseResponse and Classify are pure functions over bytes and errors, so
the taxonomy and the failure handling are covered by fixture tests that
run with no API key and no network.

Enum values are validated on the way back in: a model can return
well-formed JSON containing a difficulty of 'trivial', and storing it
would corrupt every filter and sort downstream.

Classify refuses to retry 401/403 — a wrong key does not fix itself, and
retrying quietly is how this failure mode became invisible last time.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: REST API

**Files:**
- Create: `internal/httpapi/router.go`, `internal/httpapi/router_test.go`

**Interfaces:**
- Consumes: `*ideas.Service` from Task 6, `*enrich.Enricher` from Task 7.
- Produces:
  - `httpapi.Deps{Ideas *ideas.Service; Enricher Enricher; Hub *Hub}`
  - `httpapi.Enricher` interface — `Enrich(ctx, string) (ideas.Metadata, error)`
  - `httpapi.New(deps Deps) http.Handler`
  - `(*Server).EnrichInBackground(id, rawText string)`

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/router_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/erikhoward/souschef/internal/httpapi"
	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// stubEnricher lets the HTTP tests run with no API key. Enrichment behaviour
// itself is covered by the fixture tests in internal/enrich.
type stubEnricher struct {
	meta ideas.Metadata
	err  error
}

func (s stubEnricher) Enrich(context.Context, string) (ideas.Metadata, error) {
	return s.meta, s.err
}

func newTestServer(t *testing.T, e httpapi.Enricher) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	hub := httpapi.NewHub()
	t.Cleanup(hub.Close)

	return httpapi.New(httpapi.Deps{
		Ideas:    ideas.NewService(st),
		Enricher: e,
		Hub:      hub,
	})
}

func post(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeIdea(t *testing.T, rec *httptest.ResponseRecorder) ideas.Idea {
	t.Helper()
	var out ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

func TestCreateIdeaReturnsImmediatelyAsPending(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{
		"raw_text": "Sheet-pan shawarma with a lemony feta situation",
		"source":   "web",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	got := decodeIdea(t, rec)
	if got.ID == "" {
		t.Error("response must carry the new id")
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Status = %q, want pending — the response must not wait on Claude",
			got.Enrichment.Status)
	}
	if got.Title == "" {
		t.Error("a provisional title must be present so the row is never blank")
	}
}

func TestCreateIdeaRejectsEmptyText(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{"raw_text": "  ", "source": "web"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateIdeaRejectsUnknownSource(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{"raw_text": "fine", "source": "carrier_pigeon"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown source", rec.Code)
	}
}

func TestGetIdeaNotFoundIs404(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestListIdeasReturnsArrayNotNull(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// An empty backlog must serialise as [] — null would break .map() in the UI.
	if body := rec.Body.String(); !bytes.HasPrefix([]byte(body), []byte("[")) {
		t.Errorf("empty list should serialise as [], got %s", body)
	}
}

func TestPatchRecordsOverride(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Chili eggs", "source": "web"}))

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]any{"difficulty": "easy"})
	req := httptest.NewRequest(http.MethodPatch, "/api/ideas/"+created.ID, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeIdea(t, rec)
	if got.Metadata.Difficulty != "easy" {
		t.Errorf("Difficulty = %q", got.Metadata.Difficulty)
	}
	if !got.HasOverride("difficulty") {
		t.Error("PATCH must record the field as overridden")
	}
}

func TestPatchRejectsUnknownField(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Chili eggs", "source": "web"}))

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]any{"enrichment_status": "ok"})
	req := httptest.NewRequest(http.MethodPatch, "/api/ideas/"+created.ID, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — enrichment_status is not user-correctable", rec.Code)
	}
}

func TestArchiveAndRestore(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Archivable", "source": "web"}))

	if rec := post(t, h, "/api/ideas/"+created.ID+"/archive", nil); rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d", rec.Code)
	}
	if got := decodeIdea(t, post(t, h, "/api/ideas/"+created.ID+"/restore", nil)); got.IsArchived() {
		t.Error("restore did not clear archived_at")
	}
}

func TestSearchEndpointUsesFTS(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	post(t, h, "/api/ideas", map[string]string{"raw_text": "sheet pan shawarma", "source": "web"})
	post(t, h, "/api/ideas", map[string]string{"raw_text": "cabbage soup", "source": "web"})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas?q=shawarma", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var got []ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 search result, got %d", len(got))
	}
}

func TestReenrichFailurePersistsVerbatimMessage(t *testing.T) {
	h := newTestServer(t, stubEnricher{err: errors.New("401 authentication_error: invalid x-api-key")})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Will fail", "source": "web"}))

	// /reenrich runs synchronously so the failure is observable in the response.
	rec := post(t, h, "/api/ideas/"+created.ID+"/reenrich", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}

	got := decodeIdea(t, rec)
	if got.Enrichment.Status != ideas.EnrichFailed {
		t.Errorf("Status = %q, want failed", got.Enrichment.Status)
	}
	if got.Enrichment.Error == "" {
		t.Fatal("a failure must leave a message on the row — never a silent stall")
	}
	if !bytes.Contains([]byte(got.Enrichment.Error), []byte("401")) {
		t.Errorf("Error = %q, want the provider message verbatim", got.Enrichment.Error)
	}
}

func TestReenrichSuccessAppliesMetadata(t *testing.T) {
	h := newTestServer(t, stubEnricher{meta: ideas.Metadata{
		Title: "Sheet-pan shawarma", Difficulty: "easy", DurationClass: "quick",
		Treatment: "elevated", ContentType: "recipe", Cuisine: "Middle Eastern",
		VisualPotential: "high", Seasonality: "all_year", ProductionEffort: "light",
	}})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "shawarma thing", "source": "web"}))

	got := decodeIdea(t, post(t, h, "/api/ideas/"+created.ID+"/reenrich", nil))
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Status = %q, want ok", got.Enrichment.Status)
	}
	if got.Metadata.Cuisine != "Middle Eastern" {
		t.Errorf("Cuisine = %q", got.Metadata.Cuisine)
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/httpapi/ -v`
Expected: FAIL — `undefined: httpapi.New`

- [ ] **Step 3: Write the router**

Create `internal/httpapi/router.go`:

```go
// Package httpapi exposes the REST surface and the SSE stream. It owns no SQL
// and no domain rules — every decision is delegated to internal/ideas.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// Enricher is the slice of internal/enrich this package needs. Declaring it
// here keeps HTTP tests runnable with a stub and no API key.
type Enricher interface {
	Enrich(ctx context.Context, rawText string) (ideas.Metadata, error)
}

type Deps struct {
	Ideas    *ideas.Service
	Enricher Enricher
	Hub      *Hub
}

type Server struct {
	ideas    *ideas.Service
	enricher Enricher
	hub      *Hub
}

func New(deps Deps) http.Handler {
	s := &Server{ideas: deps.Ideas, enricher: deps.Enricher, hub: deps.Hub}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ideas", s.listIdeas)
	mux.HandleFunc("POST /api/ideas", s.createIdea)
	mux.HandleFunc("GET /api/ideas/{id}", s.getIdea)
	mux.HandleFunc("PATCH /api/ideas/{id}", s.patchIdea)
	mux.HandleFunc("DELETE /api/ideas/{id}", s.deleteIdea)
	mux.HandleFunc("POST /api/ideas/{id}/archive", s.archiveIdea)
	mux.HandleFunc("POST /api/ideas/{id}/restore", s.restoreIdea)
	mux.HandleFunc("POST /api/ideas/{id}/reenrich", s.reenrichIdea)
	mux.HandleFunc("POST /api/ideas/{id}/notes", s.addNote)
	mux.HandleFunc("POST /api/ideas/{id}/links", s.addLink)
	mux.HandleFunc("DELETE /api/ideas/{id}/links/{other}", s.removeLink)
	mux.HandleFunc("POST /api/ideas/{id}/merge", s.mergeIdea)
	mux.HandleFunc("GET /events", s.hub.ServeHTTP)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httpapi: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeDomainError maps domain errors onto status codes in one place so no
// handler has to remember the mapping.
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ideas.ErrEmptyText),
		errors.Is(err, ideas.ErrTooLong),
		errors.Is(err, ideas.ErrSelfLink),
		errors.Is(err, ideas.ErrSelfMerge):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("httpapi: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return false
	}
	return true
}

var validSources = map[ideas.Source]bool{
	ideas.SourceWeb:           true,
	ideas.SourceTelegramText:  true,
	ideas.SourceTelegramVoice: true,
}

func (s *Server) createIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RawText   string       `json:"raw_text"`
		Source    ideas.Source `json:"source"`
		SourceRef string       `json:"source_ref"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Source == "" {
		body.Source = ideas.SourceWeb
	}
	if !validSources[body.Source] {
		writeError(w, http.StatusBadRequest, "unknown source: "+string(body.Source))
		return
	}

	idea, err := s.ideas.Create(r.Context(), body.RawText, body.Source, body.SourceRef)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	s.hub.Broadcast(Event{Type: "idea.created", Idea: &idea})

	// Respond now; classify in the background. Capture must never block on
	// the network — that is the property the whole design is built around.
	s.EnrichInBackground(idea.ID, idea.RawText)

	writeJSON(w, http.StatusCreated, idea)
}

// EnrichInBackground classifies an idea and pushes the result over SSE. It
// runs detached from the request, so it uses its own context.
func (s *Server) EnrichInBackground(id, rawText string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		updated, err := s.enrichOnce(ctx, id, rawText)
		if err != nil {
			log.Printf("httpapi: enrich %s: %v", id, err)
			return
		}
		s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	}()
}

// enrichOnce runs a single classification and records the outcome either way.
// A failure is a recorded state, not a dropped request.
func (s *Server) enrichOnce(ctx context.Context, id, rawText string) (ideas.Idea, error) {
	meta, err := s.enricher.Enrich(ctx, rawText)
	if err != nil {
		return s.ideas.RecordEnrichmentFailure(ctx, id, err.Error())
	}
	return s.ideas.ApplyEnrichment(ctx, id, meta, "")
}

func (s *Server) reenrichIdea(w http.ResponseWriter, r *http.Request) {
	idea, err := s.ideas.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}

	// Synchronous, unlike creation: the caller pressed Retry and is waiting
	// for an answer, so the outcome belongs in this response.
	updated, err := s.enrichOnce(r.Context(), idea.ID, idea.RawText)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) listIdeas(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	filter := ideas.ListFilter{
		Query:      q.Get("q"),
		Stage:      q.Get("stage"),
		Difficulty: q.Get("difficulty"),
		Duration:   q.Get("duration"),
		Treatment:  q.Get("treatment"),
		Archived:   ideas.ArchivedScope(q.Get("archived")),
		Sort:       q.Get("sort"),
		Order:      q.Get("order"),
		Limit:      limit,
	}

	out, err := s.ideas.List(r.Context(), filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if out == nil {
		out = []ideas.Idea{} // never serialise null — the UI calls .map on this
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getIdea(w http.ResponseWriter, r *http.Request) {
	idea, err := s.ideas.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, idea)
}

func (s *Server) patchIdea(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if !decodeBody(w, r, &patch) {
		return
	}

	updated, err := s.ideas.Correct(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		// Correct returns a plain error for an unknown or wrongly-typed field.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteIdea(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.ideas.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.deleted", ID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) archiveIdea(w http.ResponseWriter, r *http.Request)  { s.setArchived(w, r, true) }
func (s *Server) restoreIdea(w http.ResponseWriter, r *http.Request)  { s.setArchived(w, r, false) }

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	var (
		updated ideas.Idea
		err     error
	)
	if archived {
		updated, err = s.ideas.Archive(r.Context(), r.PathValue("id"))
	} else {
		updated, err = s.ideas.Restore(r.Context(), r.PathValue("id"))
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if _, err := s.ideas.AddNote(r.Context(), r.PathValue("id"), body.Body); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) addLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OtherID string `json:"other_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.ideas.Link(r.Context(), r.PathValue("id"), body.OtherID); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) removeLink(w http.ResponseWriter, r *http.Request) {
	if err := s.ideas.Unlink(r.Context(), r.PathValue("id"), r.PathValue("other")); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) mergeIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DuplicateID string `json:"duplicate_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	merged, err := s.ideas.Merge(r.Context(), r.PathValue("id"), body.DuplicateID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &merged})
	s.hub.Broadcast(Event{Type: "idea.deleted", ID: body.DuplicateID})
	writeJSON(w, http.StatusOK, merged)
}

// respondWithIdea re-reads and returns the idea, so mutations that touch
// relations always answer with fully hydrated state.
func (s *Server) respondWithIdea(w http.ResponseWriter, r *http.Request, id string) {
	idea, err := s.ideas.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &idea})
	writeJSON(w, http.StatusOK, idea)
}
```

- [ ] **Step 4: Run the tests — they will still fail on the Hub**

Run: `go test ./internal/httpapi/ -v`
Expected: FAIL — `undefined: httpapi.NewHub`. That is Task 9; continue there and return to this test.

---

## Task 9: SSE hub

**Files:**
- Create: `internal/httpapi/sse.go`, `internal/httpapi/sse_test.go`

**Interfaces:**
- Consumes: `ideas.Idea`.
- Produces: `httpapi.Event{Type string; Idea *ideas.Idea; ID string}`; `httpapi.NewHub() *Hub`; `(*Hub).Broadcast(Event)`; `(*Hub).ServeHTTP(w, r)`; `(*Hub).Close()`; `(*Hub).SubscriberCount() int`.

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/sse_test.go`:

```go
package httpapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func TestHubDeliversEventToSubscriber(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	waitForSubscribers(t, hub, 1)

	idea := ideas.Idea{ID: "i1", Title: "Crispy chili eggs"}
	hub.Broadcast(Event{Type: "idea.updated", Idea: &idea})

	got := readEvent(t, resp.Body)
	if got.Type != "idea.updated" {
		t.Errorf("Type = %q", got.Type)
	}
	if got.Idea == nil || got.Idea.Title != "Crispy chili eggs" {
		t.Errorf("payload did not survive the wire: %+v", got.Idea)
	}
}

func TestHubFansOutToMultipleSubscribers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	const n = 3
	bodies := make([]*http.Response, n)
	for i := range bodies {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("connect %d: %v", i, err)
		}
		defer resp.Body.Close()
		bodies[i] = resp
	}
	waitForSubscribers(t, hub, n)

	hub.Broadcast(Event{Type: "idea.deleted", ID: "gone"})

	for i, resp := range bodies {
		if got := readEvent(t, resp.Body); got.ID != "gone" {
			t.Errorf("subscriber %d got %+v", i, got)
		}
	}
}

func TestHubDropsSubscriberOnDisconnect(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	waitForSubscribers(t, hub, 1)

	resp.Body.Close()
	waitForSubscribers(t, hub, 0)
}

// A slow or wedged reader must not block the enrichment goroutine that is
// broadcasting. This is the failure mode that would turn one stuck browser
// tab into a stalled backend.
func TestBroadcastDoesNotBlockOnSlowSubscriber(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	waitForSubscribers(t, hub, 1)

	done := make(chan struct{})
	go func() {
		// Far more events than any per-subscriber buffer.
		for i := 0; i < 5000; i++ {
			hub.Broadcast(Event{Type: "idea.updated", ID: "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Broadcast blocked on a subscriber that stopped reading")
	}
}

func TestBroadcastIsRaceFree(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				hub.Broadcast(Event{Type: "idea.updated", ID: "x"})
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL)
			if err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			resp.Body.Close()
		}()
	}
	wg.Wait()
}

func waitForSubscribers(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("SubscriberCount = %d, want %d", hub.SubscriberCount(), want)
}

// readEvent reads one SSE frame, skipping the retry hint and any comments.
func readEvent(t *testing.T, r interface{ Read([]byte) (int, error) }) Event {
	t.Helper()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		return ev
	}
	t.Fatal("stream closed before an event arrived")
	return Event{}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/httpapi/ -run TestHub -v`
Expected: FAIL — `undefined: NewHub`

- [ ] **Step 3: Write the hub**

Create `internal/httpapi/sse.go`:

```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

// Event is what the browser receives. Idea is nil for deletions, where only
// the ID is meaningful.
type Event struct {
	Type string      `json:"type"`
	Idea *ideas.Idea `json:"idea,omitempty"`
	ID   string      `json:"id,omitempty"`
}

// subscriberBuffer is how many events a single client can fall behind before
// we start dropping its updates. Dropping is the correct trade: the client
// refetches on reconnect, whereas blocking would stall the enrichment
// goroutine doing the broadcast.
const subscriberBuffer = 64

// keepAliveInterval keeps intermediaries from closing an idle stream.
const keepAliveInterval = 25 * time.Second

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan Event]struct{})}
}

func (h *Hub) subscribe() chan Event {
	ch := make(chan Event, subscriberBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return ch
	}
	h.subscribers[ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

// Broadcast sends to every subscriber without blocking. A client that has
// stopped reading loses events rather than holding up the sender.
func (h *Hub) Broadcast(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subscribers {
		select {
		case ch <- ev:
		default:
			// Buffer full: drop. The client resynchronises on reconnect.
		}
	}
}

func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subscribers {
		delete(h.subscribers, ch)
		close(ch)
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Vite's dev proxy and any intermediary must not buffer this.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Tell the browser how quickly to come back. EventSource reconnects on
	// its own, so no client-side retry code is needed.
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case ev, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				log.Printf("httpapi: marshal event: %v", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()

		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 4: Run the whole httpapi suite**

Run: `go test ./internal/httpapi/ -race -v`
Expected: PASS — Task 8 and Task 9 tests together.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi
git commit -m "feat(httpapi): REST surface and SSE hub

Creation responds 201 with a pending idea and classifies in a detached
goroutine, so capture never blocks on the network. /reenrich is
deliberately synchronous — the caller pressed Retry and the outcome
belongs in that response.

Broadcast never blocks: a client that stops reading loses events and
resynchronises on reconnect, rather than stalling the enrichment
goroutine holding the send. Covered by a test that fires 5000 events at
a subscriber that never reads.

Empty listings serialise as [] rather than null so the UI can map over
them unguarded.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Single binary — embed the UI and wire main

**Files:**
- Create: `internal/httpapi/static.go`, `internal/httpapi/static_test.go`, `web/dist/.gitkeep`
- Modify: `cmd/souschef/main.go`

**Interfaces:**
- Consumes: everything from Tasks 2–9.
- Produces: `httpapi.WithStatic(api http.Handler) http.Handler`; a `main` that serves API and UI on one port and shuts down gracefully.

- [ ] **Step 1: Keep an embeddable directory in git**

`go:embed` fails at compile time if the directory does not exist, and `web/dist` is gitignored.

```bash
mkdir -p web/dist && touch web/dist/.gitkeep
git add -f web/dist/.gitkeep
```

- [ ] **Step 2: Write the failing test**

Create `internal/httpapi/static_test.go`:

```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erikhoward/souschef/internal/httpapi"
)

func TestStaticFallsThroughToAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := httpapi.WithStatic(api)

	req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("API routes must not be intercepted by the static handler, got %d", rec.Code)
	}
}

func TestStaticServesSPAFallbackForClientRoutes(t *testing.T) {
	h := httpapi.WithStatic(http.NotFoundHandler())

	// /ideas/<uuid> is a react-router path with no file behind it. It must
	// return index.html, not 404, or Telegram's deep links break on reload.
	req := httptest.NewRequest(http.MethodGet, "/ideas/0191f0c2-1234-7890-abcd-ef0123456789", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Error("client-side routes must fall back to index.html")
	}
}

func TestEventsRouteIsNotIntercepted(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := httpapi.WithStatic(api)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("/events must reach the API handler, got %d", rec.Code)
	}
}
```

- [ ] **Step 3: Write the static handler**

Create `internal/httpapi/static.go`:

```go
package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:../../web/dist
var distFS embed.FS

// WithStatic wraps the API handler with the embedded single-page app.
//
// Anything under /api or /events goes straight to the API. Everything else
// tries the embedded files, falling back to index.html so client-side routes
// like /ideas/<id> survive a hard reload — which is what Telegram's [Open]
// deep links rely on.
func WithStatic(api http.Handler) http.Handler {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		// Only possible if the embed directive and this path disagree.
		panic("httpapi: embedded dist not found: " + err.Error())
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/events" {
			api.ServeHTTP(w, r)
			return
		}

		if f, err := files.Open(r.URL.Path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		index, err := sub.Open("index.html")
		if err != nil {
			http.Error(w, "UI not built — run `make build`", http.StatusNotFound)
			return
		}
		defer index.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if seeker, ok := index.(interface {
			Read([]byte) (int, error)
		}); ok {
			buf := make([]byte, 0, 4096)
			tmp := make([]byte, 4096)
			for {
				n, err := seeker.Read(tmp)
				buf = append(buf, tmp[:n]...)
				if err != nil {
					break
				}
			}
			w.Write(buf)
		}
	})
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/httpapi/ -race -v`
Expected: PASS

If the embed pattern errors with "pattern outside module", create
`web/embed.go` declaring `package web` with `//go:embed all:dist` and a
`var DistFS embed.FS`, then import it from `httpapi`.

- [ ] **Step 5: Wire main**

Replace `cmd/souschef/main.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erikhoward/souschef/internal/config"
	"github.com/erikhoward/souschef/internal/enrich"
	"github.com/erikhoward/souschef/internal/httpapi"
	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "souschef %s failed: %v\n", version, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	hub := httpapi.NewHub()
	defer hub.Close()

	api := httpapi.New(httpapi.Deps{
		Ideas:    ideas.NewService(st),
		Enricher: enrich.New(cfg.Model, cfg.Effort),
		Hub:      hub,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		Handler:           httpapi.WithStatic(api),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: it would sever the SSE stream on a fixed interval.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("souschef %s listening on http://127.0.0.1:%d (db=%s, model=%s)",
			version, cfg.Port, cfg.DBPath, cfg.Model)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 6: Build and run end to end**

```bash
make build
./bin/souschef &
sleep 1
curl -s -X POST localhost:8420/api/ideas \
  -H 'Content-Type: application/json' \
  -d '{"raw_text":"sheet pan shawarma with a lemony feta situation","source":"web"}' | head -c 400
echo
curl -s localhost:8420/api/ideas | head -c 200
kill %1
```

Expected: a 201 payload with `"enrichment":{"status":"pending"}` returning
immediately, then a list containing it. With no API key configured, the row
will shortly show `"status":"failed"` with a message — that is the design
working, not a bug.

- [ ] **Step 7: Run the full suite**

Run: `go test ./... -race`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(httpapi): embed the UI and serve everything from one binary

/api/* and /events reach the API; everything else tries the embedded
files and falls back to index.html, so client-side routes survive a hard
reload — which Telegram's deep links depend on.

The server sets no WriteTimeout: a fixed one would sever the SSE stream
at a regular interval.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Retheme — Slate & Sage, Archivo + Inter

**Files:**
- Create: `web/src/assets/fonts/` (four woff2 files), `web/src/styles/tokens.css`
- Modify: `web/src/styles.css`, `web/src/main.jsx`
- Delete: `web/src/assets/recipe-thumbnails.png`

**Interfaces:**
- Consumes: nothing.
- Produces: CSS custom properties on `:root` (`--bg`, `--surface`, `--border`, `--text`, `--muted`, `--accent`, `--accent-soft`, `--on-accent`, `--font-display`, `--font-ui`); `.food-thumbnail` restyled to a deterministic mark.

- [ ] **Step 1: Vendor the fonts**

A local-first app must not fall back to Helvetica when the network drops.
Download the four weights we use and place them in `web/src/assets/fonts/`:

```bash
mkdir -p web/src/assets/fonts
cd web/src/assets/fonts
curl -fsSL -o inter-400.woff2 "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-400-normal.woff2"
curl -fsSL -o inter-500.woff2 "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-500-normal.woff2"
curl -fsSL -o inter-600.woff2 "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-600-normal.woff2"
curl -fsSL -o archivo-700.woff2 "https://cdn.jsdelivr.net/fontsource/fonts/archivo@latest/latin-700-normal.woff2"
ls -la
cd ../../../..
```

Verify each file is more than 10KB. If a URL 404s, fetch the equivalent from
`https://fontsource.org/fonts/inter` and `https://fontsource.org/fonts/archivo`
— the exact CDN path changes between releases, the fonts do not.

- [ ] **Step 2: Write the token sheet**

Create `web/src/styles/tokens.css`:

```css
/* Slate & Sage.
 *
 * The accent replaces the previous tomato red. It is not a new colour: the
 * stylesheet already used greens (#516649, #5e8761, #4d6448) for recipe and
 * review sections, so this promotes one that was already present.
 *
 * Status dots keep their own values and are always paired with a text label,
 * because the accent sits nearer the status greens than a cooler accent would.
 */

@font-face {
  font-family: 'Inter';
  src: url('../assets/fonts/inter-400.woff2') format('woff2');
  font-weight: 400; font-style: normal; font-display: swap;
}
@font-face {
  font-family: 'Inter';
  src: url('../assets/fonts/inter-500.woff2') format('woff2');
  font-weight: 500; font-style: normal; font-display: swap;
}
@font-face {
  font-family: 'Inter';
  src: url('../assets/fonts/inter-600.woff2') format('woff2');
  font-weight: 600; font-style: normal; font-display: swap;
}
@font-face {
  font-family: 'Archivo';
  src: url('../assets/fonts/archivo-700.woff2') format('woff2');
  font-weight: 700; font-style: normal; font-display: swap;
}

:root {
  --bg: #f6f3ec;
  --surface: #fcfaf5;
  --surface-alt: #f4efe6;
  --border: #dbd4c6;
  --border-strong: #c6bdac;

  --text: #24221d;
  --muted: #736c62;
  --faint: #8a8378;

  --accent: #4f6b4a;
  --accent-hover: #3f5a3b;
  --accent-soft: #eaeee4;
  --on-accent: #fbfdf8;

  --danger: #8d3023;
  --warn: #c98a2e;

  --status-idea: #8a8378;
  --status-brief: #c98a2e;
  --status-recipe: #667f61;
  --status-ready: #5e8761;
  --status-archived: #a49c90;

  --difficulty-easy: #619061;
  --difficulty-moderate: #ce9935;
  --difficulty-insane: #b04a2f;

  --font-display: 'Archivo', ui-sans-serif, system-ui, -apple-system, sans-serif;
  --font-ui: 'Inter', ui-sans-serif, system-ui, -apple-system, sans-serif;

  --radius: 6px;
  --radius-sm: 4px;
}
```

- [ ] **Step 3: Apply the tokens across styles.css**

Modify `web/src/styles.css`. Add the import at the very top:

```css
@import './styles/tokens.css';
```

Then replace every hardcoded colour with its token. The full mapping:

| Old value | Replace with |
|---|---|
| `#f5f1e8`, `#f8f5ee` | `var(--bg)` |
| `#fbfaf6`, `#fffefb`, `#fffdf8` | `var(--surface)` |
| `#f4efe6`, `#f0ebe1`, `#eee8de` | `var(--surface-alt)` |
| `#cfc7b8`, `#d7cfc1`, `#d8d0c2`, `#d1c9bb`, `#d2cabc`, `#d3ccbf` | `var(--border)` |
| `#bfb6a7`, `#bbb1a1` | `var(--border-strong)` |
| `#25231e`, `#28251f`, `#322f2a`, `#37332d`, `#36342f` | `var(--text)` |
| `#716a5f`, `#736c62`, `#746d63`, `#777168`, `#5a554d`, `#514c44`, `#514d45` | `var(--muted)` |
| `#80796f`, `#81786d`, `#847c71`, `#8b8378` | `var(--faint)` |
| `#b73826`, `#b6432c`, `#c44836`, `#bc3826`, `#c53928` | `var(--accent)` |
| `#a63020`, `#86281c`, `#852a1d` | `var(--accent-hover)` |
| `#fffaf3` (on buttons) | `var(--on-accent)` |
| `#8d3023` | `var(--danger)` |

Then replace the three font declarations:

```css
/* was: font-family: Inter, ui-sans-serif, ... */
:root { font-family: var(--font-ui); color: var(--text); background: var(--bg); }

/* was: h1, h2, .brand { font-family: Georgia, "Times New Roman", serif; font-weight: 500; } */
h1, h2, .brand {
  font-family: var(--font-display);
  font-weight: 700;
  letter-spacing: -.04em;
}

/* was: .row-title-group strong { font-family: Georgia, ...; font-weight: 500; } */
.row-title-group strong {
  display: block;
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 600;
  line-height: 1.2;
  letter-spacing: -.02em;
}

/* was: .section-title h3 { font-family: Georgia, ...; } */
.section-title h3 { font-family: var(--font-display); font-size: 17px; font-weight: 600; }

/* was: .ingredient-group h3 { font-family: Georgia, ...; font-style: italic; } */
/* This rule belongs to RecipeWorkspace, which Task 12 deletes — remove it. */

/* was: .recipe-description { font-family: Georgia, ...; } */
/* Same — remove with RecipeWorkspace. */
```

- [ ] **Step 4: Replace the thumbnail with a deterministic mark**

Delete the sprite sheet and its rule:

```bash
git rm web/src/assets/recipe-thumbnails.png
```

Replace the `.food-thumbnail` rule in `web/src/styles.css`:

```css
/* A deterministic mark derived from the primary ingredient. This is not a
 * photograph and must not pretend to be one — real cover frames arrive in
 * v1.5 when there is media to show. It preserves the row's visual rhythm
 * and gives each idea a stable, recognisable identity in the list. */
.food-thumbnail {
  width: 64px;
  height: 64px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 50%;
  background: hsl(var(--thumb-hue, 40) 32% 72%);
  color: hsl(var(--thumb-hue, 40) 45% 24%);
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  user-select: none;
}
```

- [ ] **Step 5: Wire the token sheet into the entry point**

`web/src/main.jsx` already imports `./styles.css`, which now imports the
tokens. Confirm the import order — tokens must load first:

```jsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import './styles.css';
import App from './App.jsx';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

- [ ] **Step 6: Verify the build and check visually**

```bash
cd web && bun run build && bun run dev
```

Open http://localhost:5173. Confirm: no red anywhere; the sidebar, active tab,
buttons, and "Next action" links are sage; headings and idea titles render in
Archivo; metadata rows render in Inter. Then stop the dev server.

Check for anything the mapping missed:

```bash
grep -nE '#(b7|b6|c4|bc|c5|a6|86|85)[0-9a-f]{4}' web/src/styles.css || echo "no red values remain"
```

Expected: `no red values remain`

- [ ] **Step 7: Confirm fonts are self-hosted, not fetched**

```bash
grep -rn "fonts.googleapis\|fonts.gstatic\|cdn.jsdelivr" web/src/ web/index.html || echo "no remote font references"
```

Expected: `no remote font references`

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(web): retheme to Slate & Sage with Archivo and Inter

Colours move to custom properties on :root so the palette lives in one
block. The sage accent is not a new colour — the stylesheet already used
greens for recipe and review sections; this promotes one that was there
and drops the red that was doing double duty as both 'primary button' and
'insane difficulty'.

Georgia was load-bearing: it separated editorial content from interface
chrome. Archivo's display cut takes that role so a single sans does not
flatten the hierarchy.

Fonts are vendored as woff2. The sprite sheet of fake food photography is
deleted and replaced with a deterministic mark.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: Router, API client, and live data

**Files:**
- Create: `web/src/lib/api.js`, `web/src/hooks/useIdeas.js`, `web/src/lib/thumbnail.js`
- Modify: `web/src/App.jsx`, `web/src/main.jsx`, `web/src/components/IdeasWorkspace.jsx`, `web/vite.config.js`
- Delete: `web/src/lib/pipeline.js`, `web/src/data/ideas.js`, `web/src/components/RecipeWorkspace.jsx`, `web/tests/pipeline.test.js`

**Interfaces:**
- Consumes: the REST API from Task 8, the SSE stream from Task 9.
- Produces: `api.listIdeas(params)`, `api.createIdea(rawText)`, `api.patchIdea(id, patch)`, `api.archiveIdea(id)`, `api.restoreIdea(id)`, `api.deleteIdea(id)`, `api.reenrichIdea(id)`, `api.mergeIdeas(id, dupId)`, `api.linkIdeas(id, otherId)`, `api.addNote(id, body)`; `useIdeas(filters)` returning `{ideas, loading, error, connected, create, patch, archive, restore, remove, reenrich, merge, link, addNote}`; `thumbnailProps(idea)`.

- [ ] **Step 1: Add the router and remove the mock data**

```bash
cd web
bun add react-router
bun remove --dev 2>/dev/null || true
cd ..
git rm web/src/lib/pipeline.js web/src/data/ideas.js web/src/components/RecipeWorkspace.jsx web/tests/pipeline.test.js
```

- [ ] **Step 2: Configure the dev proxy**

Create or modify `web/vite.config.js`:

```js
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8420', changeOrigin: true },
      // SSE must not be buffered by the proxy.
      '/events': { target: 'http://127.0.0.1:8420', changeOrigin: true, ws: false },
    },
  },
});
```

- [ ] **Step 3: Write the API client**

Create `web/src/lib/api.js`:

```js
// Thin wrappers over the REST surface. Every function throws an Error whose
// message is the server's, so callers can surface it verbatim rather than
// inventing their own copy.

async function request(path, { method = 'GET', body } = {}) {
  const response = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (response.status === 204) return null;

  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`Unexpected response from ${path}: ${text.slice(0, 200)}`);
    }
  }

  if (!response.ok) {
    throw new Error(payload?.error ?? `${response.status} ${response.statusText}`);
  }
  return payload;
}

export function listIdeas(params = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== '' && value != null && value !== 'all') query.set(key, value);
  }
  // `archived` is meaningful as the literal string "all", so set it explicitly.
  if (params.archived) query.set('archived', params.archived);

  const qs = query.toString();
  return request(`/api/ideas${qs ? `?${qs}` : ''}`);
}

export const createIdea = (rawText) =>
  request('/api/ideas', { method: 'POST', body: { raw_text: rawText, source: 'web' } });

export const patchIdea = (id, patch) =>
  request(`/api/ideas/${id}`, { method: 'PATCH', body: patch });

export const archiveIdea = (id) => request(`/api/ideas/${id}/archive`, { method: 'POST' });
export const restoreIdea = (id) => request(`/api/ideas/${id}/restore`, { method: 'POST' });
export const reenrichIdea = (id) => request(`/api/ideas/${id}/reenrich`, { method: 'POST' });
export const deleteIdea = (id) => request(`/api/ideas/${id}`, { method: 'DELETE' });

export const addNote = (id, body) =>
  request(`/api/ideas/${id}/notes`, { method: 'POST', body: { body } });

export const linkIdeas = (id, otherId) =>
  request(`/api/ideas/${id}/links`, { method: 'POST', body: { other_id: otherId } });

export const mergeIdeas = (id, duplicateId) =>
  request(`/api/ideas/${id}/merge`, { method: 'POST', body: { duplicate_id: duplicateId } });
```

- [ ] **Step 4: Write the deterministic thumbnail helper**

Create `web/src/lib/thumbnail.js`:

```js
// A stable mark per idea: a letter from the primary ingredient over a hue
// hashed from the same value. Deterministic, so an idea keeps its identity in
// the list across reloads.

function hashHue(input) {
  let hash = 0;
  for (let i = 0; i < input.length; i += 1) {
    hash = (hash * 31 + input.charCodeAt(i)) % 360;
  }
  return hash;
}

export function thumbnailProps(idea) {
  const seed = idea.metadata?.primary_ingredient || idea.title || idea.id;
  const letter = seed.trim().charAt(0).toUpperCase() || '·';
  return {
    letter,
    style: { '--thumb-hue': hashHue(seed) },
    label: idea.metadata?.primary_ingredient
      ? `Primary ingredient: ${idea.metadata.primary_ingredient}`
      : 'No primary ingredient inferred yet',
  };
}
```

- [ ] **Step 5: Write the data hook**

Create `web/src/hooks/useIdeas.js`:

```js
import { useCallback, useEffect, useRef, useState } from 'react';

import * as api from '../lib/api.js';

/**
 * Owns the idea list: initial fetch, mutations, and live updates over SSE.
 *
 * This replaces the fifteen-prop drill through App.jsx. Components read what
 * they need from the returned object instead of having it threaded down.
 */
export function useIdeas(filters) {
  const [ideas, setIdeas] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [connected, setConnected] = useState(false);

  // Keep the latest filters in a ref so the SSE effect does not resubscribe
  // on every keystroke in the search box.
  const filtersRef = useRef(filters);
  filtersRef.current = filters;

  const refresh = useCallback(async () => {
    try {
      setError(null);
      const rows = await api.listIdeas(filtersRef.current);
      setIdeas(rows);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setLoading(true);
    refresh();
  }, [refresh, filters.q, filters.stage, filters.difficulty,
      filters.duration, filters.treatment, filters.archived,
      filters.sort, filters.order]);

  // Live updates. EventSource reconnects on its own using the server's
  // `retry` hint, so there is no manual backoff here — but a reconnect can
  // miss events, so we refetch when the connection is re-established.
  useEffect(() => {
    const source = new EventSource('/events');
    let hasConnected = false;

    source.onopen = () => {
      setConnected(true);
      if (hasConnected) refresh(); // resynchronise after a gap
      hasConnected = true;
    };

    source.onerror = () => setConnected(false);

    source.onmessage = (event) => {
      let payload;
      try {
        payload = JSON.parse(event.data);
      } catch {
        return;
      }

      setIdeas((current) => {
        if (payload.type === 'idea.deleted') {
          return current.filter((idea) => idea.id !== payload.id);
        }
        if (!payload.idea) return current;

        const index = current.findIndex((idea) => idea.id === payload.idea.id);
        if (index === -1) {
          return payload.type === 'idea.created' ? [payload.idea, ...current] : current;
        }
        const next = current.slice();
        next[index] = payload.idea;
        return next;
      });
    };

    return () => source.close();
  }, [refresh]);

  const create = useCallback(async (rawText) => {
    // The server broadcasts idea.created, but inserting here too means the
    // row appears even if the SSE stream is momentarily down.
    const idea = await api.createIdea(rawText);
    setIdeas((current) =>
      current.some((i) => i.id === idea.id) ? current : [idea, ...current]);
    return idea;
  }, []);

  const replace = useCallback((updated) => {
    setIdeas((current) => current.map((i) => (i.id === updated.id ? updated : i)));
    return updated;
  }, []);

  const patch    = useCallback(async (id, p)     => replace(await api.patchIdea(id, p)), [replace]);
  const archive  = useCallback(async (id)        => replace(await api.archiveIdea(id)), [replace]);
  const restore  = useCallback(async (id)        => replace(await api.restoreIdea(id)), [replace]);
  const reenrich = useCallback(async (id)        => replace(await api.reenrichIdea(id)), [replace]);
  const link     = useCallback(async (id, other) => replace(await api.linkIdeas(id, other)), [replace]);
  const addNote  = useCallback(async (id, body)  => replace(await api.addNote(id, body)), [replace]);
  const merge    = useCallback(async (id, dup)   => {
    const merged = await api.mergeIdeas(id, dup);
    setIdeas((current) =>
      current.filter((i) => i.id !== dup).map((i) => (i.id === merged.id ? merged : i)));
    return merged;
  }, []);

  const remove = useCallback(async (id) => {
    await api.deleteIdea(id);
    setIdeas((current) => current.filter((i) => i.id !== id));
  }, []);

  return {
    ideas, loading, error, connected, refresh,
    create, patch, archive, restore, remove, reenrich, merge, link, addNote,
  };
}
```

- [ ] **Step 6: Rewrite App.jsx**

Replace `web/src/App.jsx`:

```jsx
import { useEffect, useRef, useState } from 'react';
import { Route, Routes, useNavigate, useParams } from 'react-router';

import { IdeasWorkspace } from './components/IdeasWorkspace.jsx';
import { Sidebar } from './components/Sidebar.jsx';
import { useIdeas } from './hooks/useIdeas.js';

const emptyFilters = {
  q: '', stage: '', difficulty: '', duration: '',
  treatment: '', archived: '', sort: 'created_at', order: 'desc',
};

function Workspace() {
  const { id: selectedId } = useParams();
  const navigate = useNavigate();

  const [filters, setFilters] = useState(emptyFilters);
  const [captureValue, setCaptureValue] = useState('');
  const [toast, setToast] = useState('');
  const toastTimer = useRef(null);

  const store = useIdeas(filters);

  const announce = (message) => {
    window.clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = window.setTimeout(() => setToast(''), 3200);
  };

  useEffect(() => () => window.clearTimeout(toastTimer.current), []);

  const guard = (fn, success) => async (...args) => {
    try {
      const result = await fn(...args);
      if (success) announce(success);
      return result;
    } catch (err) {
      announce(err.message);
      return null;
    }
  };

  const handleCreate = async (event) => {
    event.preventDefault();
    const text = captureValue.trim();
    if (!text) return;
    const idea = await guard(store.create)(text);
    if (idea) {
      setCaptureValue('');
      navigate(`/ideas/${idea.id}`);
      announce('Saved. Reading it now…');
    }
  };

  const focusCapture = () => {
    window.setTimeout(() => document.querySelector('.capture-control textarea')?.focus(), 0);
  };

  return (
    <div className="app-shell">
      <Sidebar activeView="ideas" onNavigate={() => {}} onCapture={focusCapture} />
      <IdeasWorkspace
        store={store}
        filters={filters}
        onFiltersChange={(patch) => setFilters((current) => ({ ...current, ...patch }))}
        selectedId={selectedId}
        onSelect={(id) => navigate(id ? `/ideas/${id}` : '/ideas')}
        captureValue={captureValue}
        onCaptureChange={setCaptureValue}
        onCreate={handleCreate}
        onCaptureFocus={focusCapture}
        announce={announce}
        guard={guard}
      />
      {toast && <div className="toast" role="status">{toast}</div>}
      {!store.connected && (
        <div className="connection-warning" role="status">
          Live updates disconnected — reconnecting…
        </div>
      )}
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Workspace />} />
      <Route path="/ideas" element={<Workspace />} />
      <Route path="/ideas/:id" element={<Workspace />} />
    </Routes>
  );
}
```

- [ ] **Step 7: Add the router provider**

Modify `web/src/main.jsx`:

```jsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';

import './styles.css';
import App from './App.jsx';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>
);
```

- [ ] **Step 8: Add the connection-warning style**

Append to `web/src/styles.css`:

```css
.connection-warning {
  position: fixed;
  z-index: 11;
  bottom: 22px;
  left: 24px;
  padding: 9px 13px;
  border: 1px solid var(--warn);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--muted);
  font-size: 12px;
}
```

- [ ] **Step 9: Rewire IdeasWorkspace to the store**

Modify `web/src/components/IdeasWorkspace.jsx`. Three changes; the markup and
class names are otherwise unchanged.

Replace the imports and the deleted constants at the top:

```jsx
import { useState } from 'react';

import { thumbnailProps } from '../lib/thumbnail.js';
import { Icon } from './Icon.jsx';

const STATUS_LABELS = {
  idea: 'Backlog',
  brief_ready: 'Brief ready',
  recipe_review: 'In recipe review',
  script_ready: 'Script ready',
  production_ready: 'Ready to produce',
};

const PIPELINE_STEPS = [
  ['idea', 'Idea'],
  ['brief_ready', 'Brief'],
  ['recipe_review', 'Recipe'],
  ['script_ready', 'Script'],
  ['production_ready', 'Produce'],
];

const filters = [
  ['', 'All'],
  ['idea', 'Backlog'],
  ['brief_ready', 'Brief'],
  ['recipe_review', 'Recipe'],
  ['script_ready', 'Script'],
];

const labelize = (value) => (value ? value.replaceAll('_', ' ') : '—');
```

Replace `FoodThumbnail`:

```jsx
function FoodThumbnail({ idea }) {
  const { letter, style, label } = thumbnailProps(idea);
  return (
    <span className="food-thumbnail" style={style} role="img" aria-label={label}>
      {letter}
    </span>
  );
}
```

Replace the `IdeasWorkspace` export signature and body — filtering now happens
on the server:

```jsx
export function IdeasWorkspace({
  store, filters: active, onFiltersChange, selectedId, onSelect,
  captureValue, onCaptureChange, onCreate, onCaptureFocus, announce, guard,
}) {
  const { ideas, loading, error } = store;
  const selectedIdea = ideas.find((idea) => idea.id === selectedId) ?? null;

  return (
    <main className="workspace ideas-workspace">
      <header className="workspace-header">
        <h1>Ideas</h1>
        <label className="search-box">
          <Icon name="search" size={21} />
          <input
            type="search"
            value={active.q}
            onChange={(event) => onFiltersChange({ q: event.target.value })}
            placeholder="Search ideas"
            aria-label="Search ideas"
          />
          <kbd>⌘ K</kbd>
        </label>
        <button className="button button-primary header-capture" type="button" onClick={onCaptureFocus}>
          <Icon name="plus" size={18} />Capture idea
        </button>
      </header>

      <div className="ideas-layout">
        <section className="ideas-main">
          <CaptureComposer
            captureValue={captureValue}
            onCaptureChange={onCaptureChange}
            onCapture={onCreate}
            onFocus={onCaptureFocus}
          />
          <FilterBar active={active} onChange={onFiltersChange} />

          <section className="idea-list" aria-label="Idea backlog">
            <div className="idea-list-heading">
              <span>Title</span><span /><span>Meta</span>
              <span>Content type</span><span>Status</span><span>Next action</span>
            </div>

            {error && <div className="empty-list" role="alert">{error}</div>}
            {loading && !ideas.length && <div className="empty-list">Loading…</div>}

            {ideas.map((idea) => (
              <IdeaRow
                key={idea.id}
                idea={idea}
                isSelected={idea.id === selectedId}
                onSelect={onSelect}
                onRetry={guard(store.reenrich, 'Retrying enrichment…')}
              />
            ))}

            {!loading && !ideas.length && !error && (
              <div className="empty-list">
                <Icon name="search" size={28} />
                {active.q ? 'No ideas match that search.' : 'Nothing captured yet.'}
              </div>
            )}
            <footer>{ideas.length} idea{ideas.length === 1 ? '' : 's'}</footer>
          </section>
        </section>

        {selectedIdea && (
          <IdeaInspector
            idea={selectedIdea}
            allIdeas={ideas}
            store={store}
            guard={guard}
            announce={announce}
            onSelect={onSelect}
            onClose={() => onSelect(null)}
          />
        )}
      </div>
    </main>
  );
}
```

Replace `FilterBar` so it drives the server-side filters:

```jsx
function FilterBar({ active, onChange }) {
  return (
    <div className="filter-bar">
      <div className="filter-tabs" role="tablist" aria-label="Idea stage">
        {filters.map(([id, label]) => (
          <button
            key={id || 'all'}
            type="button"
            className={active.stage === id ? 'is-active' : ''}
            onClick={() => onChange({ stage: id })}
          >
            {label}
          </button>
        ))}
        <button
          type="button"
          className={active.archived === 'true' ? 'is-active' : ''}
          onClick={() => onChange({ archived: active.archived === 'true' ? '' : 'true' })}
        >
          Archived
        </button>
      </div>
      <div className="filter-selects">
        <select value={active.difficulty} onChange={(e) => onChange({ difficulty: e.target.value })}
                aria-label="Filter by difficulty">
          <option value="">Difficulty</option>
          <option value="easy">Easy</option>
          <option value="moderate">Moderate</option>
          <option value="insane">Insane</option>
        </select>
        <select value={active.duration} onChange={(e) => onChange({ duration: e.target.value })}
                aria-label="Filter by duration">
          <option value="">Duration</option>
          <option value="quick">Quick</option>
          <option value="average">Average</option>
          <option value="multi_day">Multi-day</option>
        </select>
        <select value={active.treatment} onChange={(e) => onChange({ treatment: e.target.value })}
                aria-label="Filter by treatment">
          <option value="">Treatment</option>
          <option value="elevated">Elevated</option>
          <option value="non_elevated">Non-elevated</option>
        </select>
        <select value={active.sort} onChange={(e) => onChange({ sort: e.target.value })}
                aria-label="Sort by">
          <option value="created_at">Newest</option>
          <option value="updated_at">Recently updated</option>
          <option value="title">Title</option>
          <option value="difficulty">Difficulty</option>
          <option value="duration">Duration</option>
        </select>
        <Icon name="sliders" size={20} />
      </div>
    </div>
  );
}
```

`IdeaRow` and `IdeaInspector` are finished in Task 13.

- [ ] **Step 10: Verify against the real backend**

```bash
make build && ./bin/souschef &
sleep 1
cd web && bun run dev
```

Open http://localhost:5173, type an idea, press Save. Confirm the row appears
immediately, and that navigating to `/ideas/<id>` directly still loads. Then
stop both.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "feat(web): router, API client, and live data over SSE

Replaces useState-over-seed-fixtures with a useIdeas hook owning fetch,
mutation, and the event stream. Filtering and sorting move to the server,
so the list reflects the database rather than a client-side subset.

EventSource reconnects on its own, but a reconnect can miss events — so
the hook refetches when the stream comes back rather than trusting that
it saw everything.

Deletes pipeline.js, data/ideas.js, RecipeWorkspace.jsx and
pipeline.test.js. RecipeWorkspace returns in a later milestone built
against a real schema instead of being kept alive as a mock.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: Enrichment states and metadata correction

**Files:**
- Modify: `web/src/components/IdeasWorkspace.jsx`, `web/src/styles.css`

**Interfaces:**
- Consumes: `store` from Task 12.
- Produces: `IdeaRow` and `IdeaInspector` handling pending/failed states and inline correction.

- [ ] **Step 1: Write IdeaRow with enrichment states**

Replace `IdeaRow` in `web/src/components/IdeasWorkspace.jsx`:

```jsx
const DURATION_LABELS = { quick: 'Quick', average: 'Average', multi_day: 'Multi-day' };

function MetaIcon({ icon, children, tone = '' }) {
  return (
    <span className={`meta-value ${tone}`}>
      <Icon name={icon} size={14} />{children}
    </span>
  );
}

function IdeaRow({ idea, isSelected, onSelect, onRetry }) {
  const { status, error } = idea.enrichment;
  const meta = idea.metadata;

  return (
    <div
      className={`idea-row ${isSelected ? 'is-selected' : ''} ${status === 'failed' ? 'is-failed' : ''}`}
      role="button"
      tabIndex={0}
      onClick={() => onSelect(idea.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onSelect(idea.id);
        }
      }}
    >
      <span className="row-title-group">
        <span className="selection-mark">
          <Icon name={isSelected ? 'check' : 'lightbulb'} size={15} />
        </span>
        <span>
          <strong>{idea.title}</strong>
          <small>{idea.raw_text}</small>
        </span>
      </span>

      <FoodThumbnail idea={idea} />

      <span className="row-meta">
        {status === 'pending' && <span className="meta-pending">Reading…</span>}

        {status === 'failed' && (
          <span className="meta-failed">
            <Icon name="x" size={13} />
            <span className="meta-failed-text" title={error}>{error}</span>
            <button
              type="button"
              className="text-button"
              onClick={(event) => { event.stopPropagation(); onRetry(idea.id); }}
            >
              Retry
            </button>
          </span>
        )}

        {status === 'ok' && (
          <>
            <MetaIcon icon="lightbulb" tone={meta.difficulty}>{labelize(meta.difficulty)}</MetaIcon>
            <MetaIcon icon="clock">{DURATION_LABELS[meta.duration_class] ?? '—'}</MetaIcon>
            <MetaIcon icon="leaf">{labelize(meta.treatment)}</MetaIcon>
            {meta.cuisine && <MetaIcon icon="globe">{meta.cuisine}</MetaIcon>}
            {meta.primary_ingredient && <MetaIcon icon="egg">{meta.primary_ingredient}</MetaIcon>}
          </>
        )}
      </span>

      <span className="content-type">
        <Icon name={meta.content_type === 'vlog' ? 'video' : 'book'} size={19} />
        {meta.content_type === 'vlog' ? 'Vlog' : 'Recipe'}
      </span>

      <span className="status-cell">
        <i className={`status-dot ${idea.archived_at ? 'archived' : idea.stage}`} />
        {idea.archived_at ? 'Archived' : STATUS_LABELS[idea.stage]}
        <small>{new Date(idea.updated_at).toLocaleString()}</small>
      </span>

      <span className="row-action">
        Open<Icon name="arrow" size={18} />
      </span>
    </div>
  );
}
```

Note the element changed from `<button>` to a `<div role="button">`. The
original nested a Retry button inside an outer `<button>`, which is invalid
HTML and makes the inner control unreachable by keyboard in some browsers.

- [ ] **Step 2: Write the correction UI**

Replace `IdeaInspector` in `web/src/components/IdeasWorkspace.jsx`:

```jsx
const FIELD_OPTIONS = {
  difficulty: ['easy', 'moderate', 'insane'],
  duration_class: ['quick', 'average', 'multi_day'],
  treatment: ['elevated', 'non_elevated'],
  content_type: ['recipe', 'vlog'],
  visual_potential: ['low', 'medium', 'high'],
  seasonality: ['spring', 'summer', 'fall', 'winter', 'all_year'],
  production_effort: ['light', 'average', 'heavy'],
};

const FIELD_LABELS = {
  difficulty: 'Difficulty',
  duration_class: 'Duration',
  treatment: 'Treatment',
  content_type: 'Content type',
  cuisine: 'Cuisine',
  primary_ingredient: 'Primary ingredient',
  visual_potential: 'Visual potential',
  seasonality: 'Seasonality',
  production_effort: 'Production effort',
};

// A corrected field is marked so it is obvious which values are yours and
// which the model's — and so the protection from re-enrichment is visible
// rather than an invisible rule.
function CorrectableField({ idea, field, onCorrect }) {
  const options = FIELD_OPTIONS[field];
  const value = idea.metadata[field] ?? '';
  const overridden = idea.field_overrides.includes(field);

  return (
    <div>
      <dt>
        {FIELD_LABELS[field]}
        {overridden && <span className="override-mark" title="You set this. Re-enrichment will not change it.">✎</span>}
      </dt>
      <dd>
        {options ? (
          <select value={value} onChange={(event) => onCorrect(field, event.target.value)}
                  aria-label={FIELD_LABELS[field]}>
            <option value="">—</option>
            {options.map((option) => (
              <option key={option} value={option}>{labelize(option)}</option>
            ))}
          </select>
        ) : (
          <input
            type="text"
            defaultValue={value}
            aria-label={FIELD_LABELS[field]}
            onBlur={(event) => {
              if (event.target.value !== value) onCorrect(field, event.target.value);
            }}
          />
        )}
      </dd>
    </div>
  );
}

function PipelineStepper({ status }) {
  const index = Math.max(0, PIPELINE_STEPS.findIndex(([id]) => id === status));
  return (
    <ol className="pipeline-stepper" aria-label="Idea workflow stage">
      {PIPELINE_STEPS.map(([id, label], step) => (
        <li key={id} className={step <= index ? 'is-complete' : ''}>
          <span className={step === index ? 'is-current' : ''} />
          <small>{label}</small>
        </li>
      ))}
    </ol>
  );
}

function IdeaInspector({ idea, allIdeas, store, guard, announce, onSelect, onClose }) {
  const [noteDraft, setNoteDraft] = useState('');
  const [linkTarget, setLinkTarget] = useState('');
  const [mergeTarget, setMergeTarget] = useState('');

  const correct = guard(
    (field, value) => store.patch(idea.id, { [field]: value }),
    'Saved. Re-enrichment will leave that field alone.'
  );

  const candidates = allIdeas.filter(
    (other) => other.id !== idea.id && !idea.linked_ids.includes(other.id));

  return (
    <aside className="inspector" aria-label={`${idea.title} details`}>
      <button className="icon-button inspector-close" type="button" onClick={onClose}
              aria-label="Close details">
        <Icon name="x" size={20} />
      </button>

      <h2>{idea.title}</h2>
      <PipelineStepper status={idea.stage} />

      {idea.enrichment.status === 'failed' && (
        <div className="inspector-alert" role="alert">
          <strong>Enrichment failed</strong>
          <p>{idea.enrichment.error}</p>
          <button className="button button-outline" type="button"
                  onClick={() => guard(store.reenrich, 'Retrying…')(idea.id)}>
            Retry
          </button>
        </div>
      )}

      <section className="inspector-section metadata-section">
        <div className="section-title"><h3>Metadata</h3></div>
        <dl className="metadata-list">
          {Object.keys(FIELD_LABELS).map((field) => (
            <CorrectableField key={field} idea={idea} field={field} onCorrect={correct} />
          ))}
        </dl>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Original capture</h3></div>
        <p className="muted-copy raw-text">{idea.raw_text}</p>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Notes</h3></div>
        {idea.notes.length > 0 && (
          <ul className="note-list">
            {idea.notes.map((note) => <li key={note.id}>{note.body}</li>)}
          </ul>
        )}
        <form
          className="link-control"
          onSubmit={async (event) => {
            event.preventDefault();
            if (!noteDraft.trim()) return;
            if (await guard(store.addNote, 'Note added.')(idea.id, noteDraft)) setNoteDraft('');
          }}
        >
          <input type="text" value={noteDraft} placeholder="Add a note…"
                 aria-label="New note" onChange={(event) => setNoteDraft(event.target.value)} />
          <button className="text-button" type="submit" disabled={!noteDraft.trim()}>Add</button>
        </form>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Related ideas</h3></div>
        {idea.linked_ids.length === 0 ? (
          <p className="muted-copy">No linked ideas yet.</p>
        ) : (
          <div className="related-list">
            {idea.linked_ids.map((id) => {
              const other = allIdeas.find((i) => i.id === id);
              return (
                <button key={id} type="button" onClick={() => onSelect(id)}>
                  <span>{other ? other.title : id}</span>
                </button>
              );
            })}
          </div>
        )}
        {candidates.length > 0 && (
          <div className="link-control">
            <select value={linkTarget} onChange={(event) => setLinkTarget(event.target.value)}
                    aria-label="Link another idea">
              <option value="">Link another idea…</option>
              {candidates.map((c) => <option key={c.id} value={c.id}>{c.title}</option>)}
            </select>
            <button className="text-button" type="button" disabled={!linkTarget}
                    onClick={async () => {
                      if (await guard(store.link, 'Linked.')(idea.id, linkTarget)) setLinkTarget('');
                    }}>
              Link
            </button>
          </div>
        )}
      </section>

      <div className="inspector-actions">
        {candidates.length > 0 && (
          <div className="link-control">
            <select value={mergeTarget} onChange={(event) => setMergeTarget(event.target.value)}
                    aria-label="Merge a duplicate into this idea">
              <option value="">Merge a duplicate in…</option>
              {candidates.map((c) => <option key={c.id} value={c.id}>{c.title}</option>)}
            </select>
            <button className="text-button" type="button" disabled={!mergeTarget}
                    onClick={async () => {
                      if (await guard(store.merge, 'Merged.')(idea.id, mergeTarget)) setMergeTarget('');
                    }}>
              Merge
            </button>
          </div>
        )}

        <div className="secondary-actions">
          <button type="button" className="text-button"
                  onClick={() => guard(idea.archived_at ? store.restore : store.archive,
                                       idea.archived_at ? 'Restored.' : 'Archived.')(idea.id)}>
            <Icon name={idea.archived_at ? 'arrow' : 'archive'} size={15} />
            {idea.archived_at ? 'Restore' : 'Archive'}
          </button>
          <button type="button" className="text-button delete-button"
                  onClick={() => {
                    if (window.confirm(`Delete “${idea.title}”? This cannot be undone.`)) {
                      guard(store.remove, 'Deleted.')(idea.id);
                      onClose();
                    }
                  }}>
            <Icon name="trash" size={15} />Delete
          </button>
        </div>
      </div>
    </aside>
  );
}
```

- [ ] **Step 3: Add the styles for the new states**

Append to `web/src/styles.css`:

```css
.meta-pending { color: var(--faint); font-size: 11px; font-style: italic; }

.meta-failed {
  display: flex; align-items: center; gap: 6px;
  color: var(--danger); font-size: 11px;
}
.meta-failed-text {
  display: block; max-width: 190px;
  overflow: hidden; white-space: nowrap; text-overflow: ellipsis;
}
.idea-row.is-failed { background: color-mix(in srgb, var(--danger) 4%, transparent); }

.inspector-alert {
  margin: 16px 0; padding: 13px;
  border: 1px solid var(--danger); border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--danger) 6%, var(--surface));
}
.inspector-alert strong { display: block; color: var(--danger); font-size: 12px; }
.inspector-alert p {
  margin: 6px 0 11px;
  color: var(--muted); font-size: 11px; line-height: 1.4;
  word-break: break-word;
}

.override-mark { margin-left: 5px; color: var(--accent); font-size: 11px; }

.metadata-list select,
.metadata-list input[type='text'] {
  width: 100%; height: 28px; padding: 0 7px;
  border: 1px solid var(--border); border-radius: 3px;
  background: var(--surface); color: var(--text); font-size: 11px;
}

.note-list { margin: 11px 0 0; padding-left: 17px; }
.note-list li { margin-bottom: 6px; color: var(--muted); font-size: 11px; line-height: 1.4; }

.raw-text { white-space: pre-wrap; }

.idea-row { cursor: pointer; }
.idea-row:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
```

- [ ] **Step 4: Verify by hand against a failing key**

```bash
make build
env ANTHROPIC_API_KEY=sk-ant-definitely-invalid ./bin/souschef &
sleep 1
cd web && bun run dev
```

Capture an idea. Confirm: it appears instantly, shows "Reading…", then within
seconds flips to a red row showing a `401` message with a working Retry
button. **This is the acceptance test for done-definition item 8** — if the
row spins forever or shows nothing, stop and fix it before continuing.

Then stop both processes.

- [ ] **Step 5: Verify the happy path**

Repeat with a valid credential. Confirm metadata chips fill in without a
refresh, and that changing a dropdown shows the ✎ mark and survives a Retry.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(web): enrichment states and inline metadata correction

Pending shows a placeholder; failed shows the provider's message on the
row with a working Retry, and marks the row so it is visible in a long
list. This is the fix for the failure mode where a dead key looked
identical to slow work.

Corrected fields carry a ✎ mark, so which values are yours and which the
model's is visible rather than an invisible rule about re-enrichment.

IdeaRow becomes a div[role=button]: the original nested a Retry button
inside an outer button, which is invalid HTML and left the inner control
unreachable by keyboard in some browsers.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: whisper.cpp transcription

**Files:**
- Create: `internal/transcribe/whisper.go`, `internal/transcribe/whisper_test.go`, `internal/transcribe/testdata/sample.ogg`
- Modify: `README.md`

**Interfaces:**
- Consumes: `config.Config.WhisperBin`, `.WhisperModel`, `.AudioDir`.
- Produces: `transcribe.New(bin, model string) *Transcriber`; `(*Transcriber).Transcribe(ctx, audioPath string) (string, error)`; `transcribe.ErrEmptyTranscript`.

- [ ] **Step 1: Record a short sample**

```bash
mkdir -p internal/transcribe/testdata
# macOS: record ~3 seconds saying "sheet pan shawarma with lemony feta"
ffmpeg -f avfoundation -i ":default" -t 3 -c:a libopus -b:a 24k \
  internal/transcribe/testdata/sample.ogg
ls -la internal/transcribe/testdata/
```

If `ffmpeg` is unavailable, any short spoken `.ogg` works. Keep it under 100KB.

- [ ] **Step 2: Write the failing test**

Create `internal/transcribe/whisper_test.go`:

```go
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

	got, err := tr.Transcribe(ctx, filepath.Join("testdata", "sample.ogg"))
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

	_, err := tr.Transcribe(context.Background(), "testdata/does-not-exist.ogg")
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

	if _, err := tr.Transcribe(ctx, filepath.Join("testdata", "sample.ogg")); err == nil {
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
```

- [ ] **Step 3: Run it to make sure it fails**

Run: `go test ./internal/transcribe/ -v`
Expected: FAIL — `undefined: New` (or SKIP messages plus the `stripTimestamps` failure)

- [ ] **Step 4: Write the implementation**

Create `internal/transcribe/whisper.go`:

```go
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
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/transcribe/ -v`

Expected with whisper installed: PASS.
Expected without: SKIP on the three integration tests with an actionable
message, PASS on both `stripTimestamps` tests.

If the flags are rejected, run `whisper-cli --help` and adjust — the binary is
named `main` in some builds and `whisper-cli` in Homebrew's.

- [ ] **Step 6: Document the prerequisite**

Replace `README.md`:

```markdown
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
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(transcribe): whisper.cpp wrapper for voice notes

Local by design: Claude does not accept audio, so transcription is a
separate component either way, and running it locally costs nothing per
note and keeps recordings on the machine.

Integration tests skip with an actionable install message when whisper is
absent — it is a documented prerequisite, not a vendored dependency, and
a fresh checkout must not fail on it. stripTimestamps is pure and stays
covered regardless.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 15: Telegram client and command registry

**Files:**
- Create: `internal/telegram/client.go`, `internal/telegram/commands.go`, `internal/telegram/commands_test.go`

**Interfaces:**
- Consumes: `config.Config.TelegramToken`, `.TelegramChatID`.
- Produces:
  - `telegram.Client` with `GetUpdates`, `SendMessage`, `EditMessageText`, `AnswerCallbackQuery`, `GetFile`, `DownloadFile`, `GetMyCommands`, `SetMyCommands`
  - `telegram.Command{Name, Args, Desc string; Handler HandlerFunc}`
  - `telegram.Commands []Command` — the single source of truth
  - `telegram.ValidateRegistry([]Command) error`
  - `telegram.MenuFrom([]Command) []BotCommand`
  - `telegram.HelpText([]Command) string`
  - `telegram.Lookup(name string) (Command, bool)`

- [ ] **Step 1: Write the failing test**

Create `internal/telegram/commands_test.go`:

```go
package telegram

import (
	"regexp"
	"strings"
	"testing"
)

// Telegram's own constraints. A malformed entry must fail at startup rather
// than at the setMyCommands call.
var telegramName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func TestRegistryMeetsTelegramConstraints(t *testing.T) {
	if err := ValidateRegistry(Commands); err != nil {
		t.Fatalf("the shipped registry must be valid: %v", err)
	}

	for _, c := range Commands {
		if !telegramName.MatchString(c.Name) {
			t.Errorf("name %q must be 1-32 chars of [a-z0-9_]", c.Name)
		}
		if n := len(c.Desc); n < 3 || n > 256 {
			t.Errorf("description for /%s is %d chars, must be 3-256", c.Name, n)
		}
	}
}

// The invariant the design is built on: a command in the menu with no handler
// must be impossible. Handler is a func field with no usable zero value, so
// this is really a belt-and-braces check on the shipped slice.
func TestEveryCommandHasAHandler(t *testing.T) {
	for _, c := range Commands {
		if c.Handler == nil {
			t.Errorf("/%s appears in the registry with no handler", c.Name)
		}
	}
}

func TestValidateRegistryRejectsBadEntries(t *testing.T) {
	cases := map[string][]Command{
		"uppercase name":   {{Name: "Search", Desc: "Search ideas", Handler: noopHandler}},
		"name with space":  {{Name: "find idea", Desc: "Search ideas", Handler: noopHandler}},
		"empty name":       {{Name: "", Desc: "Search ideas", Handler: noopHandler}},
		"short desc":       {{Name: "s", Desc: "hi", Handler: noopHandler}},
		"missing handler":  {{Name: "s", Desc: "Search ideas"}},
		"duplicate name": {
			{Name: "s", Desc: "Search ideas", Handler: noopHandler},
			{Name: "s", Desc: "Search again", Handler: noopHandler},
		},
	}

	for label, registry := range cases {
		if err := ValidateRegistry(registry); err == nil {
			t.Errorf("%s should have been rejected", label)
		}
	}
}

func TestLookupFindsRegisteredCommands(t *testing.T) {
	for _, c := range Commands {
		if _, ok := Lookup(c.Name); !ok {
			t.Errorf("Lookup(%q) failed for a registered command", c.Name)
		}
	}
	if _, ok := Lookup("definitely_not_a_command"); ok {
		t.Error("Lookup must not invent commands")
	}
}

// The menu published to Telegram is derived from the registry, never written
// by hand — that is what makes drift impossible.
func TestMenuIsDerivedFromRegistry(t *testing.T) {
	menu := MenuFrom(Commands)

	if len(menu) != len(Commands) {
		t.Fatalf("menu has %d entries, registry has %d", len(menu), len(Commands))
	}
	for i, c := range Commands {
		if menu[i].Command != c.Name {
			t.Errorf("menu[%d].Command = %q, want %q", i, menu[i].Command, c.Name)
		}
		if menu[i].Description != c.Desc {
			t.Errorf("menu[%d].Description = %q, want %q", i, menu[i].Description, c.Desc)
		}
		// Args is documentation for /help only — Telegram has no parameter
		// syntax, and including it would show literal "<query>" in the menu.
		if strings.Contains(menu[i].Command, "<") {
			t.Errorf("menu[%d].Command leaked Args: %q", i, menu[i].Command)
		}
	}
}

// /help must cover exactly the registry: no missing entries, no extras.
func TestHelpTextCoversExactlyTheRegistry(t *testing.T) {
	help := HelpText(Commands)

	for _, c := range Commands {
		if !strings.Contains(help, "/"+c.Name) {
			t.Errorf("/help omits /%s", c.Name)
		}
		if !strings.Contains(help, c.Desc) {
			t.Errorf("/help omits the description for /%s", c.Name)
		}
		if c.Args != "" && !strings.Contains(help, c.Args) {
			t.Errorf("/help omits the argument hint %q for /%s", c.Args, c.Name)
		}
	}

	// Count slash-prefixed lines to catch extras that are not in the registry.
	lines := 0
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "/") {
			lines++
		}
	}
	if lines != len(Commands) {
		t.Errorf("/help lists %d commands, registry has %d", lines, len(Commands))
	}
}

func TestMenusEqualIgnoresOrder(t *testing.T) {
	a := []BotCommand{{Command: "s", Description: "Search"}, {Command: "help", Description: "Help"}}
	b := []BotCommand{{Command: "help", Description: "Help"}, {Command: "s", Description: "Search"}}

	if !menusEqual(a, b) {
		t.Error("menus differing only in order must compare equal — otherwise every restart writes to the API")
	}
	if menusEqual(a, []BotCommand{{Command: "s", Description: "Search ideas"}}) {
		t.Error("a genuine difference must be detected")
	}
}
```

Add the no-op handler used by the rejection cases:

```go
func noopHandler(*Bot, Update, string) error { return nil }
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/telegram/ -v`
Expected: FAIL — `undefined: Commands`

- [ ] **Step 3: Write the client**

Create `internal/telegram/client.go`:

```go
// Package telegram implements the bot.
//
// There is no SDK here on purpose: we need seven Bot API methods, all plain
// JSON over HTTPS. A small client avoids inheriting a dependency's release
// cadence and keeps the long-poll timeout under our control.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const apiBase = "https://api.telegram.org"

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		// Longer than the long-poll timeout below, so getUpdates is never cut
		// off by the transport.
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", method, err)
		}
		body = bytes.NewReader(buf)
	}

	url := fmt.Sprintf("%s/bot%s/%s", apiBase, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	var envelope apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if !envelope.OK {
		return fmt.Errorf("%s failed: %d %s", method, envelope.ErrorCode, envelope.Description)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// --- wire types (only the fields we use) ---

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
	Voice     *Voice `json:"voice"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// --- methods ---

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	var out []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	}, &out)
	return out, err
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string,
	markup *InlineKeyboardMarkup) (Message, error) {

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}

	var out Message
	err := c.call(ctx, "sendMessage", payload, &out)
	return out, err
}

func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string,
	markup *InlineKeyboardMarkup) error {

	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return c.call(ctx, "editMessageText", payload, nil)
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery",
		map[string]any{"callback_query_id": id, "text": text}, nil)
}

func (c *Client) GetMyCommands(ctx context.Context, chatID int64) ([]BotCommand, error) {
	var out []BotCommand
	err := c.call(ctx, "getMyCommands", map[string]any{
		"scope": map[string]any{"type": "chat", "chat_id": chatID},
	}, &out)
	return out, err
}

// SetMyCommands publishes the menu, scoped to the one chat allowed to use the
// bot rather than to everyone who finds it.
func (c *Client) SetMyCommands(ctx context.Context, chatID int64, commands []BotCommand) error {
	return c.call(ctx, "setMyCommands", map[string]any{
		"commands": commands,
		"scope":    map[string]any{"type": "chat", "chat_id": chatID},
	}, nil)
}

// DownloadFile resolves a file_id and writes the bytes into dir, returning the
// local path.
func (c *Client) DownloadFile(ctx context.Context, fileID, dir string) (string, error) {
	var meta struct {
		FilePath string `json:"file_path"`
	}
	if err := c.call(ctx, "getFile", map[string]any{"file_id": fileID}, &meta); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/file/bot%s/%s", apiBase, c.token, meta.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", fileID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", fileID, resp.StatusCode)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// filepath.Base guards against a traversal via the server-supplied path.
	dest := filepath.Join(dir, fileID+filepath.Ext(filepath.Base(meta.FilePath)))

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}
	return dest, nil
}
```

- [ ] **Step 4: Write the registry**

Create `internal/telegram/commands.go`:

```go
package telegram

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// HandlerFunc handles one command. args is everything after the command word.
type HandlerFunc func(bot *Bot, update Update, args string) error

// Command binds a command's name, its user-facing description, and its
// handler in one place.
type Command struct {
	Name    string      // as typed, without the leading slash
	Args    string      // documentation for /help only; Telegram has no parameter syntax
	Desc    string      // shown in Telegram's command menu
	Handler HandlerFunc
}

// Commands is the single source of truth for what this bot can do.
//
// Three consumers read this slice and nothing else: the menu published via
// setMyCommands, the update router, and /help. BotFather is never used to
// configure commands — it only issues the token.
//
// Because Handler has no usable zero value, a menu entry with no handler
// cannot be written without failing validation at startup.
var Commands = []Command{
	{
		Name:    "s",
		Args:    "<query>",
		Desc:    "Search your ideas",
		Handler: handleSearch,
	},
	{
		Name:    "recent",
		Desc:    "Show your ten newest ideas",
		Handler: handleRecent,
	},
	{
		Name:    "help",
		Desc:    "What this bot can do",
		Handler: handleHelp,
	},
}

var namePattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// ValidateRegistry checks the registry against Telegram's constraints so a
// malformed entry fails at startup rather than at the API call.
func ValidateRegistry(registry []Command) error {
	seen := map[string]bool{}

	for _, c := range registry {
		if !namePattern.MatchString(c.Name) {
			return fmt.Errorf("command %q: name must be 1-32 characters of lowercase letters, digits, and underscores", c.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("command %q is registered twice", c.Name)
		}
		seen[c.Name] = true

		if n := len(c.Desc); n < 3 || n > 256 {
			return fmt.Errorf("command %q: description is %d characters, must be 3-256", c.Name, n)
		}
		if c.Handler == nil {
			return fmt.Errorf("command %q has no handler", c.Name)
		}
	}
	return nil
}

func Lookup(name string) (Command, bool) {
	for _, c := range Commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// MenuFrom derives the Telegram menu. Args is deliberately excluded — it is
// documentation for /help, and Telegram would render it literally.
func MenuFrom(registry []Command) []BotCommand {
	out := make([]BotCommand, 0, len(registry))
	for _, c := range registry {
		out = append(out, BotCommand{Command: c.Name, Description: c.Desc})
	}
	return out
}

// HelpText renders /help from the same slice, so the two can never disagree.
func HelpText(registry []Command) string {
	var b strings.Builder
	b.WriteString("Send me any message to capture it as an idea. A voice note works too.\n\n")

	for _, c := range registry {
		b.WriteString("/" + c.Name)
		if c.Args != "" {
			b.WriteString(" " + c.Args)
		}
		b.WriteString(" — " + c.Desc + "\n")
	}
	return b.String()
}

// menusEqual compares order-independently, so a restart that changed nothing
// does not write to a rate-limited API.
func menusEqual(a, b []BotCommand) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(list []BotCommand) []string {
		out := make([]string, 0, len(list))
		for _, c := range list {
			out = append(out, c.Command+"\x00"+c.Description)
		}
		sort.Strings(out)
		return out
	}
	left, right := key(a), key(b)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// SyncCommands publishes the menu only when it differs from what Telegram
// already has, turning a real change into one log line and a no-op restart
// into nothing at all.
func SyncCommands(ctx context.Context, client *Client, chatID int64, registry []Command) (bool, error) {
	if err := ValidateRegistry(registry); err != nil {
		return false, err
	}

	want := MenuFrom(registry)

	current, err := client.GetMyCommands(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("read current command menu: %w", err)
	}
	if menusEqual(current, want) {
		return false, nil
	}
	if err := client.SetMyCommands(ctx, chatID, want); err != nil {
		return false, fmt.Errorf("publish command menu: %w", err)
	}
	return true, nil
}
```

Add `"context"` to the import block.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/telegram/ -v`

Expected: FAIL on `undefined: handleSearch` and friends — those arrive in Task
16. Comment out the three registry entries temporarily to confirm the
validation, menu, and help tests pass, then restore them and move on.

- [ ] **Step 6: Commit**

```bash
git add internal/telegram
git commit -m "feat(telegram): API client and command registry

No SDK: seven Bot API methods over plain JSON, so we do not inherit a
dependency's release cadence and the long-poll timeout stays ours.

The registry binds name, description, and handler in one slice, and the
menu, the router, and /help are all derived from it. Handler has no
usable zero value, so a menu entry with no handler fails validation at
startup. BotFather only issues the token.

SyncCommands compares against getMyCommands first: a no-op restart writes
nothing to a rate-limited API, and a real change produces one log line.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 16: Telegram capture — text and voice

**Files:**
- Create: `internal/telegram/bot.go`, `internal/telegram/bot_test.go`
- Modify: `cmd/souschef/main.go`

**Interfaces:**
- Consumes: everything from Tasks 6, 7, 14, 15.
- Produces: `telegram.Bot`; `telegram.New(deps Deps) (*Bot, error)`; `(*Bot).Run(ctx) error`; `telegram.Deps{Client, Ideas, Enricher, Transcriber, ChatID, AudioDir, WebBaseURL}`; `telegram.RenderIdeaCard(idea) (string, *InlineKeyboardMarkup)`.

- [ ] **Step 1: Write the failing test**

Create `internal/telegram/bot_test.go`:

```go
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func enrichedIdea() ideas.Idea {
	at := time.Now()
	return ideas.Idea{
		ID:      "0191f0c2-1234-7890-abcd-ef0123456789",
		Title:   "Crispy chili eggs with scallion oil",
		RawText: "chili crisp eggs, scallion oil, fast",
		Stage:   ideas.StageIdea,
		Metadata: ideas.Metadata{
			Difficulty: "easy", DurationClass: "quick",
			Treatment: "elevated", Cuisine: "Chinese-inspired",
		},
		Enrichment: ideas.Enrichment{Status: ideas.EnrichOK, EnrichedAt: &at},
	}
}

func TestRenderIdeaCardShowsMetadata(t *testing.T) {
	text, markup := RenderIdeaCard(enrichedIdea(), "http://localhost:8420")

	for _, want := range []string{"Crispy chili eggs", "Easy", "Quick", "Elevated", "Chinese-inspired"} {
		if !strings.Contains(text, want) {
			t.Errorf("card omits %q:\n%s", want, text)
		}
	}
	if markup == nil || len(markup.InlineKeyboard) == 0 {
		t.Fatal("an enriched card must carry an Open button")
	}
}

// No ID is ever shown, copied, or pasted. That was the defect in the previous
// iteration and it must not reappear.
func TestRenderIdeaCardNeverShowsRawID(t *testing.T) {
	idea := enrichedIdea()
	text, _ := RenderIdeaCard(idea, "http://localhost:8420")

	if strings.Contains(text, idea.ID) {
		t.Errorf("card leaks the raw id into visible text:\n%s", text)
	}
}

func TestRenderIdeaCardDeepLinksToTheWebApp(t *testing.T) {
	idea := enrichedIdea()
	_, markup := RenderIdeaCard(idea, "http://localhost:8420")

	var found string
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.URL != "" {
				found = button.URL
			}
		}
	}
	want := "http://localhost:8420/ideas/" + idea.ID
	if found != want {
		t.Errorf("deep link = %q, want %q", found, want)
	}
}

func TestRenderIdeaCardPendingSaysReading(t *testing.T) {
	idea := enrichedIdea()
	idea.Enrichment = ideas.Enrichment{Status: ideas.EnrichPending}

	text, _ := RenderIdeaCard(idea, "http://localhost:8420")
	if !strings.Contains(strings.ToLower(text), "reading") {
		t.Errorf("a pending card should say it is still reading:\n%s", text)
	}
}

func TestRenderIdeaCardFailedShowsErrorAndRetry(t *testing.T) {
	idea := enrichedIdea()
	idea.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichFailed,
		Error:  "401 authentication_error: invalid x-api-key",
	}

	text, markup := RenderIdeaCard(idea, "http://localhost:8420")
	if !strings.Contains(text, "401") {
		t.Errorf("a failed card must show the message:\n%s", text)
	}

	var hasRetry bool
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.Contains(strings.ToLower(button.Text), "retry") {
				hasRetry = true
			}
		}
	}
	if !hasRetry {
		t.Error("a failed card must offer Retry")
	}
}

func TestParseCommandSplitsNameAndArgs(t *testing.T) {
	cases := []struct{ in, name, args string }{
		{"/s shawarma", "s", "shawarma"},
		{"/s  lots   of  words ", "s", "lots   of  words"},
		{"/recent", "recent", ""},
		{"/help", "help", ""},
		{"/s@souschef_bot shawarma", "s", "shawarma"}, // group-style mention
		{"not a command", "", ""},
		{"", "", ""},
	}

	for _, c := range cases {
		name, args := parseCommand(c.in)
		if name != c.name || args != c.args {
			t.Errorf("parseCommand(%q) = (%q, %q), want (%q, %q)", c.in, name, args, c.name, c.args)
		}
	}
}

func TestEscapeHTMLPreventsMarkupInjection(t *testing.T) {
	// An idea title is user-supplied and lands in an HTML-parsed message.
	got := escapeHTML(`<b>bold</b> & "quoted"`)
	for _, forbidden := range []string{"<b>", "</b>"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("escapeHTML left %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("escapeHTML did not escape &: %q", got)
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `go test ./internal/telegram/ -run TestRender -v`
Expected: FAIL — `undefined: RenderIdeaCard`

- [ ] **Step 3: Write the bot**

Create `internal/telegram/bot.go`:

```go
package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

type Enricher interface {
	Enrich(ctx context.Context, rawText string) (ideas.Metadata, error)
}

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

type Deps struct {
	Client      *Client
	Ideas       *ideas.Service
	Enricher    Enricher
	Transcriber Transcriber
	ChatID      int64
	AudioDir    string
	WebBaseURL  string
}

type Bot struct {
	Deps
	offset int64
}

func New(deps Deps) (*Bot, error) {
	if err := ValidateRegistry(Commands); err != nil {
		return nil, err
	}
	return &Bot{Deps: deps}, nil
}

const longPollSeconds = 30

// Run polls for updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	// Publish the menu, but never let a sync failure stop the bot: a stale
	// menu costs discoverability, whereas refusing to run costs capture.
	changed, err := SyncCommands(ctx, b.Client, b.ChatID, Commands)
	switch {
	case err != nil:
		log.Printf("telegram: command menu not synced: %v", err)
	case changed:
		log.Printf("telegram: published %d commands", len(Commands))
	default:
		log.Printf("telegram: command menu already current")
	}

	log.Printf("telegram: listening (chat %d)", b.ChatID)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.Client.GetUpdates(ctx, b.offset, longPollSeconds)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("telegram: getUpdates: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			b.offset = update.UpdateID + 1
			if err := b.handle(ctx, update); err != nil {
				log.Printf("telegram: handle update %d: %v", update.UpdateID, err)
			}
		}
	}
}

// authorised drops anything from a chat that is not the configured one.
func (b *Bot) authorised(update Update) bool {
	switch {
	case update.Message != nil:
		return update.Message.Chat.ID == b.ChatID
	case update.CallbackQuery != nil && update.CallbackQuery.Message != nil:
		return update.CallbackQuery.Message.Chat.ID == b.ChatID
	default:
		return false
	}
}

func (b *Bot) handle(ctx context.Context, update Update) error {
	if !b.authorised(update) {
		log.Printf("telegram: dropped update from an unauthorised chat")
		return nil
	}

	if update.CallbackQuery != nil {
		return b.handleCallback(ctx, update)
	}
	if update.Message == nil {
		return nil
	}
	if update.Message.Voice != nil {
		return b.handleVoice(ctx, update)
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	if name, args := parseCommand(text); name != "" {
		cmd, ok := Lookup(name)
		if !ok {
			// An unknown command answers with help rather than being
			// silently swallowed.
			_, err := b.Client.SendMessage(ctx, b.ChatID,
				"I don't know /"+escapeHTML(name)+".\n\n"+HelpText(Commands), nil)
			return err
		}
		return cmd.Handler(b, update, args)
	}

	return b.capture(ctx, text, ideas.SourceTelegramText,
		fmt.Sprint(update.Message.MessageID))
}

// parseCommand splits "/name args" into its parts. It tolerates the
// "/name@botname" form Telegram uses in groups.
func parseCommand(text string) (name, args string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	body := strings.TrimPrefix(text, "/")

	head, rest, _ := strings.Cut(body, " ")
	if head == "" {
		return "", ""
	}
	if at := strings.IndexByte(head, '@'); at >= 0 {
		head = head[:at]
	}
	return head, strings.TrimSpace(rest)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// capture saves an idea, replies immediately, and enriches in the background,
// editing the same message in place when the result arrives.
func (b *Bot) capture(ctx context.Context, rawText string, source ideas.Source, ref string) error {
	idea, err := b.Ideas.Create(ctx, rawText, source, ref)
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID, "Could not save that: "+escapeHTML(err.Error()), nil)
		return sendErr
	}

	sent, err := b.Client.SendMessage(ctx, b.ChatID,
		"✓ Saved. <i>Reading it now…</i>", nil)
	if err != nil {
		return err
	}

	go b.enrichAndEdit(idea.ID, idea.RawText, sent.MessageID)
	return nil
}

// enrichAndEdit classifies the idea and rewrites the original message.
// Editing in place, rather than sending a second message, keeps the chat at
// one message per idea — which matters when the chat is the phone-side view
// of the backlog.
func (b *Bot) enrichAndEdit(ideaID, rawText string, messageID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var updated ideas.Idea

	meta, err := b.Enricher.Enrich(ctx, rawText)
	if err != nil {
		updated, err = b.Ideas.RecordEnrichmentFailure(ctx, ideaID, err.Error())
	} else {
		updated, err = b.Ideas.ApplyEnrichment(ctx, ideaID, meta, "")
	}
	if err != nil {
		log.Printf("telegram: record enrichment for %s: %v", ideaID, err)
		return
	}

	text, markup := RenderIdeaCard(updated, b.WebBaseURL)
	if err := b.Client.EditMessageText(ctx, b.ChatID, messageID, text, markup); err != nil {
		log.Printf("telegram: edit message %d: %v", messageID, err)
	}
}

func (b *Bot) handleVoice(ctx context.Context, update Update) error {
	sent, err := b.Client.SendMessage(ctx, b.ChatID, "🎙 <i>Transcribing…</i>", nil)
	if err != nil {
		return err
	}

	path, err := b.Client.DownloadFile(ctx, update.Message.Voice.FileID, b.AudioDir)
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not download that voice note: "+escapeHTML(err.Error()), nil)
	}

	text, err := b.Transcriber.Transcribe(ctx, path)
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not transcribe that: "+escapeHTML(err.Error()), nil)
	}

	idea, err := b.Ideas.Create(ctx, text, ideas.SourceTelegramVoice,
		fmt.Sprint(update.Message.MessageID))
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not save that: "+escapeHTML(err.Error()), nil)
	}

	if err := b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
		"✓ Saved: <i>"+escapeHTML(text)+"</i>\n\n<i>Reading it now…</i>", nil); err != nil {
		return err
	}

	go b.enrichAndEdit(idea.ID, idea.RawText, sent.MessageID)
	return nil
}

var difficultyLabels = map[string]string{"easy": "Easy", "moderate": "Moderate", "insane": "Insane"}
var durationLabels = map[string]string{"quick": "Quick", "average": "Average", "multi_day": "Multi-day"}
var treatmentLabels = map[string]string{"elevated": "Elevated", "non_elevated": "Everyday"}

// RenderIdeaCard builds the message body and keyboard for one idea. The id
// appears only inside callback_data and the deep link, never in visible text.
func RenderIdeaCard(idea ideas.Idea, webBaseURL string) (string, *InlineKeyboardMarkup) {
	var b strings.Builder
	b.WriteString("<b>" + escapeHTML(idea.Title) + "</b>\n")

	buttons := []InlineKeyboardButton{}

	switch idea.Enrichment.Status {
	case ideas.EnrichPending:
		b.WriteString("<i>Reading it now…</i>")

	case ideas.EnrichFailed:
		b.WriteString("⚠️ Could not read it: " + escapeHTML(idea.Enrichment.Error))
		buttons = append(buttons, InlineKeyboardButton{
			Text: "Retry", CallbackData: "retry:" + idea.ID,
		})

	default:
		parts := []string{}
		for _, v := range []string{
			difficultyLabels[idea.Metadata.Difficulty],
			durationLabels[idea.Metadata.DurationClass],
			treatmentLabels[idea.Metadata.Treatment],
			idea.Metadata.Cuisine,
		} {
			if v != "" {
				parts = append(parts, escapeHTML(v))
			}
		}
		b.WriteString(strings.Join(parts, " · "))
	}

	if webBaseURL != "" {
		buttons = append(buttons, InlineKeyboardButton{
			Text: "Open", URL: webBaseURL + "/ideas/" + idea.ID,
		})
	}

	if len(buttons) == 0 {
		return b.String(), nil
	}
	return b.String(), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{buttons}}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/telegram/ -run 'TestRender|TestParse|TestEscape' -v`
Expected: PASS. The registry tests still fail on the missing handlers; Task 17
adds them.

- [ ] **Step 5: Commit**

```bash
git add internal/telegram
git commit -m "feat(telegram): capture by text and voice with edit-in-place

Saving replies immediately and enrichment rewrites the same message via
editMessageText, so the chat stays at one message per idea and mirrors
the SSE behaviour in the web app.

Cards never render a raw id — it lives only in callback_data and the deep
link. Titles are HTML-escaped because they are user-supplied text landing
in an HTML-parsed message.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 17: Telegram search, callbacks, wiring, and end-to-end

**Files:**
- Create: `internal/telegram/handlers.go`, `web/tests/capture.spec.js`, `web/playwright.config.js`
- Modify: `cmd/souschef/main.go`, `PROJECT_LOG.md`

**Interfaces:**
- Consumes: everything.
- Produces: `handleSearch`, `handleRecent`, `handleHelp`, `(*Bot).handleCallback`; a Playwright suite; a merged branch.

- [ ] **Step 1: Write the handlers**

Create `internal/telegram/handlers.go`:

```go
package telegram

import (
	"context"
	"strings"

	"github.com/erikhoward/souschef/internal/ideas"
)

const maxResults = 5

// resultsKeyboard turns a result set into tappable buttons. This is the whole
// point of the redesign: the previous iteration made you copy and paste ids.
func resultsKeyboard(results []ideas.Idea) *InlineKeyboardMarkup {
	rows := make([][]InlineKeyboardButton, 0, len(results))
	for _, idea := range results {
		label := idea.Title
		if len(label) > 48 {
			label = label[:47] + "…"
		}
		rows = append(rows, []InlineKeyboardButton{{
			Text:         label,
			CallbackData: "open:" + idea.ID,
		}})
	}
	if len(rows) == 0 {
		return nil
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func handleSearch(b *Bot, update Update, args string) error {
	ctx := context.Background()

	query := strings.TrimSpace(args)
	if query == "" {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Give me something to search for — for example: <code>/s shawarma</code>", nil)
		return err
	}

	results, err := b.Ideas.List(ctx, ideas.ListFilter{Query: query, Limit: maxResults})
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID,
			"Search failed: "+escapeHTML(err.Error()), nil)
		return sendErr
	}
	if len(results) == 0 {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Nothing matches “"+escapeHTML(query)+"”.", nil)
		return err
	}

	_, err = b.Client.SendMessage(ctx,
		b.ChatID,
		"Tap one to open it:",
		resultsKeyboard(results))
	return err
}

func handleRecent(b *Bot, update Update, _ string) error {
	ctx := context.Background()

	results, err := b.Ideas.List(ctx, ideas.ListFilter{Sort: "created_at", Order: "desc", Limit: 10})
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID,
			"Could not load your backlog: "+escapeHTML(err.Error()), nil)
		return sendErr
	}
	if len(results) == 0 {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Nothing captured yet. Send me anything and I'll save it.", nil)
		return err
	}

	_, err = b.Client.SendMessage(ctx, b.ChatID, "Your ten newest:", resultsKeyboard(results))
	return err
}

func handleHelp(b *Bot, update Update, _ string) error {
	_, err := b.Client.SendMessage(context.Background(), b.ChatID, HelpText(Commands), nil)
	return err
}

// handleCallback services the inline keyboard. Callbacks are not commands:
// they arrive as callback_data, never as typed text, and Telegram has no menu
// concept for them — which is why they are not in the registry.
func (b *Bot) handleCallback(ctx context.Context, update Update) error {
	query := update.CallbackQuery
	action, id, found := strings.Cut(query.Data, ":")
	if !found {
		return b.Client.AnswerCallbackQuery(ctx, query.ID, "")
	}

	switch action {
	case "open":
		idea, err := b.Ideas.Get(ctx, id)
		if err != nil {
			return b.Client.AnswerCallbackQuery(ctx, query.ID, "That idea is gone.")
		}
		if err := b.Client.AnswerCallbackQuery(ctx, query.ID, ""); err != nil {
			return err
		}
		text, markup := RenderIdeaCard(idea, b.WebBaseURL)
		_, err = b.Client.SendMessage(ctx, b.ChatID, text, markup)
		return err

	case "retry":
		idea, err := b.Ideas.Get(ctx, id)
		if err != nil {
			return b.Client.AnswerCallbackQuery(ctx, query.ID, "That idea is gone.")
		}
		if err := b.Client.AnswerCallbackQuery(ctx, query.ID, "Retrying…"); err != nil {
			return err
		}
		if _, err := b.Ideas.MarkPending(ctx, id); err != nil {
			return err
		}
		go b.enrichAndEdit(idea.ID, idea.RawText, query.Message.MessageID)
		return nil

	default:
		return b.Client.AnswerCallbackQuery(ctx, query.ID, "")
	}
}
```

- [ ] **Step 2: Run the full telegram suite**

Run: `go test ./internal/telegram/ -race -v`
Expected: PASS — including every registry test from Task 15.

- [ ] **Step 3: Wire the bot into main**

Modify `cmd/souschef/main.go`. Add the imports and, after the HTTP server
goroutine, before `<-ctx.Done()`:

```go
	telegramClient := telegram.NewClient(cfg.TelegramToken)
	bot, err := telegram.New(telegram.Deps{
		Client:      telegramClient,
		Ideas:       ideaService,
		Enricher:    enricher,
		Transcriber: transcribe.New(cfg.WhisperBin, cfg.WhisperModel),
		ChatID:      cfg.TelegramChatID,
		AudioDir:    cfg.AudioDir,
		WebBaseURL:  fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
	})
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}

	go func() {
		if err := bot.Run(ctx); err != nil {
			log.Printf("telegram: %v", err)
		}
	}()
```

This requires hoisting `ideaService` and `enricher` into named variables where
the `httpapi.Deps` literal currently builds them inline:

```go
	ideaService := ideas.NewService(st)
	enricher := enrich.New(cfg.Model, cfg.Effort)

	api := httpapi.New(httpapi.Deps{
		Ideas:    ideaService,
		Enricher: enricher,
		Hub:      hub,
	})
```

- [ ] **Step 4: Build and verify the Telegram loop by hand**

```bash
make build && ./bin/souschef
```

Expected in the log: `telegram: published 3 commands` on the first run, then
`telegram: command menu already current` on every subsequent start. In the
Telegram app, typing `/` must show the three commands **without any BotFather
configuration** — this is done-definition item 5.

Then, in the chat:
1. Send `sheet pan shawarma with lemony feta` → "✓ Saved. Reading it now…" then the same message becomes a metadata card with an Open button.
2. Send a voice note → "🎙 Transcribing…" → the transcript → the card.
3. Send `/s shawarma` → tappable results. Tap one; it opens without you seeing an ID.
4. Tap Open → the browser lands on `/ideas/<id>` and the idea is selected.

- [ ] **Step 5: Write the Playwright config**

Create `web/playwright.config.js`:

```js
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  use: { baseURL: 'http://127.0.0.1:8420', trace: 'retain-on-failure' },
  // Test the real binary, not the dev server: the embedded-asset path and
  // the SPA fallback only exist in the built artifact.
  webServer: {
    command: '../bin/souschef',
    url: 'http://127.0.0.1:8420',
    reuseExistingServer: !process.env.CI,
    timeout: 20_000,
  },
});
```

- [ ] **Step 6: Write the end-to-end spec**

Create `web/tests/capture.spec.js`:

```js
import { expect, test } from '@playwright/test';

// The core loop from the definition of done. Enrichment needs a real
// credential, so the metadata assertion is conditional — but capture,
// search, archive, and restore must pass unconditionally.
const hasCredential = Boolean(process.env.ANTHROPIC_API_KEY);

test('capture appears immediately and is searchable', async ({ page }) => {
  await page.goto('/ideas');

  const unique = `shawarma-${Date.now()}`;
  await page.fill('.capture-control textarea', `sheet pan ${unique} with lemony feta`);
  await page.click('button:has-text("Save idea")');

  // Instant: the row must not wait on the network.
  await expect(page.locator('.idea-row').first()).toContainText(unique, { timeout: 2000 });

  await page.fill('input[aria-label="Search ideas"]', unique);
  await expect(page.locator('.idea-row')).toHaveCount(1);
});

test('archive hides the idea and restore brings it back', async ({ page }) => {
  await page.goto('/ideas');

  const unique = `archivable-${Date.now()}`;
  await page.fill('.capture-control textarea', unique);
  await page.click('button:has-text("Save idea")');
  await expect(page.locator('.idea-row').first()).toContainText(unique);

  await page.click('.idea-row:first-child');
  await page.click('button:has-text("Archive")');
  await expect(page.locator('.idea-row', { hasText: unique })).toHaveCount(0);

  await page.click('button:has-text("Archived")');
  await expect(page.locator('.idea-row', { hasText: unique })).toHaveCount(1);

  await page.click('.idea-row:first-child');
  await page.click('button:has-text("Restore")');
});

test('a deep link to a single idea loads directly', async ({ page }) => {
  await page.goto('/ideas');

  const unique = `deeplink-${Date.now()}`;
  await page.fill('.capture-control textarea', unique);
  await page.click('button:has-text("Save idea")');
  await expect(page.locator('.idea-row').first()).toContainText(unique);

  // The URL became /ideas/<id> on save. A hard reload must still work —
  // this is what Telegram's Open button relies on.
  const url = page.url();
  expect(url).toMatch(/\/ideas\/[0-9a-f-]{36}$/);

  await page.goto(url);
  await expect(page.locator('.inspector')).toContainText(unique);
});

test('a correction is marked and persists across a reload', async ({ page }) => {
  await page.goto('/ideas');

  const unique = `correctable-${Date.now()}`;
  await page.fill('.capture-control textarea', unique);
  await page.click('button:has-text("Save idea")');
  await expect(page.locator('.idea-row').first()).toContainText(unique);
  await page.click('.idea-row:first-child');

  await page.selectOption('select[aria-label="Difficulty"]', 'insane');
  await expect(page.locator('.override-mark')).toHaveCount(1);

  await page.reload();
  await expect(page.locator('select[aria-label="Difficulty"]')).toHaveValue('insane');
});

test.describe('with a live credential', () => {
  test.skip(!hasCredential, 'ANTHROPIC_API_KEY not set');

  test('metadata fills in over SSE without a refresh', async ({ page }) => {
    await page.goto('/ideas');

    const unique = `enriched-${Date.now()}`;
    await page.fill('.capture-control textarea',
      `crispy chili eggs with scallion oil ${unique}, quick weeknight thing`);
    await page.click('button:has-text("Save idea")');

    const row = page.locator('.idea-row').first();
    await expect(row).toContainText('Reading');
    // No reload anywhere in this test — SSE must deliver it.
    await expect(row.locator('.meta-value').first()).toBeVisible({ timeout: 30_000 });
  });
});
```

- [ ] **Step 7: Install Playwright and run the suite**

```bash
cd web
bun add -d @playwright/test
bunx playwright install chromium
cd .. && make build
cd web && bunx playwright test
```

Expected: four tests pass, one skipped without a credential.

- [ ] **Step 8: Run everything**

```bash
make test
```

Expected: all Go packages pass with `-race`, all Playwright tests pass.

- [ ] **Step 9: Walk the definition of done**

Check each item from the spec against the running app, and fix anything that
fails before merging:

1. `make build` produces one binary serving UI and API on one port.
2. Web capture is on screen in under 50ms, enriched within ~5s, no refresh.
3. Same from Telegram, text and voice.
4. `/s <query>` returns tappable results — no IDs.
5. The command menu is populated by the app; `/help` matches it.
6. List, search, filter, sort, notes, links, merge, archive, restore, delete.
7. Every inferred field is correctable, and correcting protects it.
8. A dead key shows a visible error with a working Retry.
9. `make test` passes.
10. `PROJECT_LOG.md` is current.

- [ ] **Step 10: Update PROJECT_LOG.md**

Set version to `1.0.0`, move Milestone 1 to **Complete**, and add to the Log:

```markdown
**2026-08-06** — Milestone 1 complete and merged. Go backend with SQLite +
FTS5, Claude Sonnet 5 enrichment, single binary with embedded React and SSE,
Telegram capture by text and voice with tappable search. Record here whether
prompt caching engaged (`cache_read_input_tokens` non-zero on a second run)
and the observed enrichment latency.
```

- [ ] **Step 11: Commit and merge**

```bash
git add -A
git commit -m "feat(telegram): tappable search, callbacks, and end-to-end suite

Search returns an inline keyboard keyed by callback_data, so an idea is
opened by tapping it. No id is ever displayed or pasted — the defect that
made the Telegram-first iteration unusable.

Callbacks stay out of the command registry: they arrive as callback_data,
never as typed text, and Telegram has no menu concept for them.

Playwright runs against the built binary rather than the dev server,
because the embedded assets and the SPA fallback only exist there.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"

git checkout main
git merge --no-ff feat/milestone-1-capture -m "Merge Milestone 1: capture and organize

Capture from web or Telegram by text or voice, Claude-inferred metadata
arriving over SSE without a refresh, and a searchable, correctable
backlog. Single binary, local-first.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push origin main
```

- [ ] **Step 12: Stop for approval**

Milestone 1 is complete. Do not begin Milestone 2 (brief generation) until
Erik has reviewed and approved.

---

## Self-Review

Checked against the spec on 2026-08-06.

**Spec coverage.** Every section maps to a task: §3 repo layout → Task 1; §3
package boundaries → Tasks 2–9; §4 data model → Tasks 3–6; §5 capture and
enrichment → Tasks 7–8; §6 HTTP API → Tasks 8–9; §7 Telegram → Tasks 14–17;
§8 frontend → Tasks 11–13; §9 testing → distributed across every task plus
Task 17; §10 configuration → Task 2; §11 definition of done → Task 17 Step 9.

**Deliberate omission.** The spec's §6 lists `DELETE /api/notes/:id`. No task
implements it — the inspector has no delete-note control, so wiring the
endpoint would ship dead code. Note deletion belongs with note editing in a
later milestone. Everything else in §6 is covered.

**Type consistency.** `ideas.Idea`, `ideas.Metadata`, `ideas.ListFilter`, and
`store.ErrNotFound` are defined in Tasks 4 and 6 and used with the same
signatures in Tasks 8, 15, 16, and 17. `httpapi.Enricher` and
`telegram.Enricher` are separate one-method interfaces over the same
`enrich.Enricher` — deliberate, so each package declares only what it needs
and both stay testable with a stub.

**One thing removed during review.** An earlier draft seeded a deliberate typo
into the Task 11 token sheet as a "read this, don't paste it" check. That was
wrong: a plan is meant to be executed faithfully, and a malformed CSS custom
property fails silently in the browser. Anything worth catching belongs in a
test, not in a trap. Removed.

**Known sharp edges for the implementer.** Three places where the first
attempt is likely to fail and the plan says so inline rather than pretending
otherwise: the `go:embed` relative paths in Tasks 3 and 10 (fallback shape
given), the Anthropic Go SDK type names in Task 7 (compile-fix loop, not
research), and whisper.cpp's flag names in Task 14, which differ between the
Homebrew build and a source build.

| 8 | `httpapi`: REST routes and JSON |
| 9 | `httpapi`: SSE hub with reconnect |
| 10 | `httpapi`: embed `web/dist`, single-binary serving, graceful shutdown |
| 11 | `web`: retheme — CSS custom properties, Slate & Sage, self-hosted Archivo + Inter |
| 12 | `web`: router, `useIdeas` hook, delete mock data, wire list to API |
| 13 | `web`: enrichment states, retry control, metadata correction UI |
| 14 | `transcribe`: whisper.cpp wrapper with skipping test |
| 15 | `telegram`: HTTP client and command registry with menu sync |
| 16 | `telegram`: capture (text + voice), edit-in-place enrichment |
| 17 | `telegram`: search, callbacks, Playwright e2e, PROJECT_LOG, merge |
