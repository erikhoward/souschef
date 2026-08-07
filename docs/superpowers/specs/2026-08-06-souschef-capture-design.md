# Sous Chef — Milestone 1: Capture & Organize

**Date:** 2026-08-06
**Status:** Approved design, ready for implementation planning
**Scope:** Milestone 1 of the recipe content pipeline — idea capture and organization, web + Telegram, with real persistence and real inference. Stops before brief generation.

---

## 1. Context

This is the fourth iteration of Sous Chef. The prior three each failed in a specific, informative way:

1. **Petal Labs toolchain** — premature; the toolchain was solving problems the product didn't have yet.
2. **Telegram-first** — the *capture* experience was right (voice and natural-language text, instant, on a phone), but retrieval was unusable: finding an existing idea meant copy-pasting IDs, and editing in Telegram was painful.
3. **Codex-generated Vite + React app** — visually excellent, functionally hollow. No backend, no agent, no Telegram, and the web project was placed at the repo root, leaving no room for a server.

The two validated insights carried forward:

- **Rapid capture via voice or loose natural language is the core value.** It has to be instant and it has to work from a phone.
- **Seeing the backlog at a glance — what exists, and whether each item is still an idea, or has become a brief or a recipe — is the second-most valuable thing.** This is what Telegram could not do.

Milestone 1 delivers exactly those two things, done properly, and nothing else.

### Disposition of the generated React app

Assessed in full (1,136 lines).

| Piece | Lines | Disposition |
|---|---|---|
| `src/styles.css` | 288 | **Keep, refactored.** Hand-written CSS, no framework. A real layout system: sidebar + workspace grid, list + sticky inspector rail, recipe-document layout, four considered breakpoints down to 760px. Retheming is a variable swap. |
| `src/components/*.jsx` | 352 | **Keep as visual scaffold, rewire completely.** Markup and class structure are sound; every data path is fabricated. |
| `src/App.jsx` | 128 | **Discard.** All state in `useState` over seed data, prop-drilled 15 deep. No router. |
| `src/lib/pipeline.js` | 152 | **Discard.** `inferCuisine()` is a regex ladder standing where a Claude call belongs. |
| `src/data/ideas.js` | 123 | **Discard.** Seed fixtures. |
| `src/assets/recipe-thumbnails.png` | — | **Discard.** A five-frame CSS sprite sheet faking food photography. |
| `tests/pipeline.test.js` | 82 | **Discard.** Tests the fabricated logic. |

Roughly 25% is reusable, and it is the 25% that carries the look and feel.

The clearest evidence it is a mockup rather than an application: `advanceIdea()` rewrites a status string and sets `updatedAt: 'Just now'` — a literal string, not a timestamp. Nothing is generated and nothing persists.

---

## 2. Decisions

### 2.1 Milestone boundary

Milestone 1 is **capture and organize, done for real, including Telegram.** Repo restructure, Go backend, SQLite persistence, real Claude metadata inference, full ideas CRUD, search, filter, sort, notes, links, merge, archive/restore/delete, the retheme, and Telegram text + voice capture and search.

It explicitly stops before brief generation. Briefs, recipes, review chat, scripts, exports, and the producer agent are later milestones, each with its own spec.

Rationale: three iterations have stalled on scope. This slice is the smallest thing that is genuinely useful on the day it ships, and it is the thing already validated twice.

### 2.2 Stack

- **Backend: Go.** The milestone is CRUD + HTTP + a Telegram long-poll loop + Claude calls. Go is fast to iterate on, compiles to a single static binary, and makes "save instantly, enrich in the background" a five-line pattern. Mature Telegram and Anthropic SDKs.
- **Frontend: React + Vite**, carried over from the generated app.
- **Storage: SQLite, single file.** The portability requirement in the original brief is about *export* (JSON, Markdown, Schema.org JSON-LD), which is a later milestone and orthogonal to storage engine. A document store buys nothing for a single user and costs a daemon. One `souschef.db` is copyable to a thumb drive.
- **Transcription: whisper.cpp, local.** Claude models do not accept audio, so transcription is a separate component regardless. A local binary honors "local only for v1.0" literally, costs nothing per note, has no quota to exhaust, and keeps voice recordings on the machine. It ships as a documented prerequisite validated at startup, not a hidden dependency.
- **No authentication on the web app.** Binds to localhost, single user. Telegram gets a hard chat-ID allowlist, because that surface is actually exposed.

### 2.3 Architecture: single binary, embedded React, SSE push

Go serves the API *and* the built React app via `embed.FS`. One process, one binary, no reverse proxy. In development, Vite runs separately and proxies `/api` and `/events` to Go, preserving hot reload. Enrichment results push to the browser over Server-Sent Events.

Alternatives considered and rejected:

- **Polling instead of SSE.** Cheaper to build, worse to live with: any poll interval is either laggy or wasteful, and "did it work?" ambiguity during enrichment is precisely the failure mode that made a prior rate-limit incident hard to diagnose.
- **Synchronous enrichment** (save blocks on the Claude call). Simplest possible, and fatal here — it puts a multi-second spinner on every capture and fails capture entirely when the API is unavailable, breaking the one property that has to hold.

SSE rather than WebSockets because the traffic is strictly server→client; the client only ever asks via normal REST. SSE is plain HTTP, reconnects automatically in the browser with no client code, and in Go is a `Flusher` plus a channel.

### 2.4 Visual direction

- **Palette: Slate & Sage.** Warm paper retained (`#f6f3ec` / `#fcfaf5`), tomato-soup red replaced with deep sage `#4f6b4a`. The existing stylesheet already runs a green sub-palette (`#516649` on recipe headings, `#5e8761` on easy difficulty, `#4d6448` on review headers), so this promotes a color already present rather than introducing one. Accepted tradeoff: sage sits nearer the status-dot greens than a cooler accent would; status dots keep their own distinct values and are always paired with a text label.
- **Typography: Archivo for display, Inter for UI and body.** Georgia was load-bearing in the original — it separated editorial content (recipe titles, descriptions) from interface chrome (buttons, labels, metadata). Replacing it with a single sans flattens that hierarchy. Archivo's display cut restores the separation; Inter handles the 11px metadata that fills every table row, where it is best in class.
- Fonts are **self-hosted** as woff2 in `web/src/assets/fonts/`. A local-first app must not degrade to system fallback when the network drops.

---

## 3. Repository layout

The web app moves out of the root; Go owns the root because Go is the shipped artifact.

```
souschef/
├── cmd/souschef/main.go        # single entrypoint: API + web + telegram
├── internal/
│   ├── config/                 # env loading, validation, startup checks
│   ├── store/                  # SQLite: connection, migrations, all queries
│   ├── ideas/                  # domain: create, correct, search, merge, link, archive
│   ├── enrich/                 # Claude client, prompt, schema validation
│   ├── transcribe/             # whisper.cpp wrapper
│   ├── telegram/               # long-poll loop, command handlers
│   └── httpapi/                # routes, JSON, SSE hub, embedded web/dist
├── migrations/                 # 0001_init.sql, ...
├── web/                        # Vite + React
│   ├── src/
│   └── dist/                   # built; embedded into the binary
├── docs/superpowers/specs/
├── PROJECT_LOG.md
├── Makefile                    # dev / build / test
└── .env.example
```

- `make dev` — Go and Vite side by side, Vite proxying `/api` and `/events`.
- `make build` — `bun run build` then `go build`, producing one binary with the UI inside it.
- `make test` — Go tests plus the Playwright spec.

The same binary is what goes into a container when cloud deployment is wanted later. Nothing about the design changes; only where it runs.

### Package boundaries

| Package | Owns | Notes |
|---|---|---|
| `config` | Env loading and startup validation | Fails fast and loudly |
| `store` | SQLite connection, migrations, **all SQL** | No other package writes queries |
| `ideas` | Domain rules | Takes a `store`, returns domain errors |
| `enrich` | Claude call, schema validation, retry | **Never touches the database** |
| `transcribe` | whisper.cpp subprocess | Returns text or a typed error |
| `telegram` | Long-poll loop, command routing | Calls `ideas` and `enrich`; owns no SQL |
| `httpapi` | Routes, JSON, SSE hub, embedded assets | Calls `ideas`; owns no SQL |

Two boundaries are enforced deliberately:

- **Only `store` writes SQL.** Telegram and web therefore cannot drift on what "archive" means.
- **`enrich` is a pure function from text to metadata.** This makes the expensive, nondeterministic, key-requiring part of the system testable offline against recorded fixtures.

---

## 4. Data model

Seven tables plus an FTS index.

```sql
ideas
  id                    TEXT PRIMARY KEY      -- UUIDv7, time-sortable
  title                 TEXT NOT NULL         -- short display title (derived, editable)
  raw_text              TEXT NOT NULL         -- exactly what was said/typed; never rewritten
  source                TEXT NOT NULL         -- web | telegram_text | telegram_voice
  source_ref            TEXT                  -- telegram message id, for traceability
  stage                 TEXT NOT NULL         -- idea | brief_ready | recipe_review | script_ready | production_ready
  archived_at           DATETIME              -- NULL = active
  merged_into_id        TEXT                  -- tombstone; NULL = not merged
  -- inferred metadata, all NULL until enrichment lands
  difficulty            TEXT                  -- easy | moderate | insane
  duration_class        TEXT                  -- quick | average | multi_day
  treatment             TEXT                  -- elevated | non_elevated
  content_type          TEXT                  -- recipe | vlog
  cuisine               TEXT
  primary_ingredient    TEXT
  equipment             TEXT                  -- JSON array
  visual_potential      TEXT                  -- low | medium | high
  seasonality           TEXT                  -- spring | summer | fall | winter | all_year
  production_effort     TEXT                  -- light | average | heavy
  field_overrides       TEXT NOT NULL DEFAULT '[]'   -- JSON: ["difficulty","cuisine"]
  -- enrichment bookkeeping
  enrichment_status     TEXT NOT NULL         -- pending | ok | failed
  enrichment_error      TEXT                  -- verbatim error text
  enrichment_model      TEXT                  -- model id that produced this
  enriched_at           DATETIME
  created_at            DATETIME NOT NULL
  updated_at            DATETIME NOT NULL

notes        (id, idea_id, body, created_at)
tags         (id, name UNIQUE)
idea_tags    (idea_id, tag_id)
idea_links   (idea_a_id, idea_b_id)          -- canonical ordering a<b, no duplicates
transcripts  (id, idea_id, audio_path, text, duration_ms, created_at)
ideas_fts    -- FTS5 virtual table over title, raw_text, cuisine, primary_ingredient, tags
```

### Rationale for the non-obvious choices

**`stage` and `archived_at` are separate columns.** In the generated app, archiving overwrites `status` with `'archived'` and restoring sets it back to `'idea'` — silently destroying the fact that an item had reached brief stage. Archiving is orthogonal to pipeline position, so it gets its own column.

**`raw_text` is immutable and distinct from `title`.** What was dictated into a phone at 11pm is evidence. The title is a derived, editable label. `raw_text` is also what re-enrichment reads.

`title` is populated in two stages: at capture it is `raw_text` truncated at the first sentence boundary or 60 characters, so the row is never blank; enrichment then replaces it with a model-generated title. `title` participates in `field_overrides` like any other inferred field — edit it once and re-enrichment leaves it alone.

**`field_overrides` records human corrections.** Without it, "allow correction of inferred metadata" and "retry enrichment" are in direct conflict — re-enrichment would clobber corrections. Re-enrichment writes only fields absent from this list.

**`merged_into_id` is a tombstone, not a delete.** Merging duplicates leaves the losing row pointing at the winner, so an old Telegram message or an existing link still resolves.

**FTS5 is the answer to the Telegram search problem.** SQLite's full-text engine, kept in sync by triggers, gives ranked prefix and substring matching in well under a millisecond at this data size. That is what makes Telegram search return *tappable results* instead of IDs to copy-paste. The original pain was never a Telegram limitation; it was the absence of a search index, which pushed identity resolution onto the human.

**Enrichment bookkeeping is four columns, not a boolean.** Status says what happened, error says why, model says which, timestamp says when. The timestamp matters more than it appears: when the model is changed later, every idea enriched by the previous one can be found and re-run without a full rescan.

### Semantics

- **Archive** is reversible and preserves `stage`. **Delete** is permanent, behind a confirmation.
- **Link** is symmetric, stored once with canonical ordering; self-links are rejected.
- **Merge** unions notes, tags, and links onto the primary and tombstones the duplicate.

---

## 5. Capture and enrichment

### Capture path — never touches the network

```
POST /api/ideas {raw_text, source}
  → validate (non-empty, ≤5000 chars)
  → INSERT with enrichment_status='pending'      ~2ms
  → 201 Created, full row returned               ← client is done here
  → go enrichIdea(id)                            (background goroutine)
```

### Enrichment path — background

```
enrich.Enrich(ctx, raw_text)
  → model: claude-sonnet-5 (configurable)
  → output_config.format = JSON schema covering title + the 11 metadata fields
  → output_config.effort = "low"; adaptive thinking
  → system prompt (taxonomy definitions) marked cache_control: ephemeral
  → 60s timeout; 2 retries on 429/5xx with exponential backoff
  ↓ success                                ↓ failure
UPDATE ideas SET …, status='ok'       UPDATE ideas SET status='failed',
                                        enrichment_error=<verbatim>
  ↓                                          ↓
     SSE broadcast {type: "idea.updated", idea: {…}}
```

**Model choice.** The original brief scoped "frontier Claude models only" to *recipe generation*, which is a later milestone. Metadata inference from a short text is an easier task; Sonnet 5 with structured outputs is well matched to it and the cost delta versus a smaller model is immaterial at a few ideas per day. The model is a config value, not a constant — `claude-opus-5` is a one-line change.

**Structured output** uses `output_config.format` with a JSON schema. The deprecated top-level `output_format` parameter is not used. Exact Go SDK type names are settled against the compiler during implementation rather than guessed.

### Failure handling

The Go SDK returns a single `*anthropic.Error` for all non-2xx responses:

```go
var apierr *anthropic.Error
if errors.As(err, &apierr) {
    switch apierr.StatusCode {
    case 401, 403: // bad or unauthorized key — do not retry, surface immediately
    case 429:      // no quota or rate limited — retry twice, then fail visibly
    default:       // record StatusCode and RequestID in enrichment_error
    }
}
```

Whatever lands in `enrichment_error` is displayed verbatim on the idea row alongside a working **Retry** control. A dead or unkeyed API key therefore reads as `401 authentication_error` on the row within seconds, rather than as a spinner that never resolves. The idea itself remains fully usable throughout — the captured text was never at risk.

**Startup validation** in `config` is the companion fix: on boot the process verifies that the Anthropic credential resolves, the whisper binary and model file exist and are executable, and the Telegram token is valid. Any missing prerequisite prints a named error and the process refuses to start.

### Prompt caching note

The enrichment system prompt (taxonomy definitions — what distinguishes `insane` from `moderate`, what counts as `elevated`) is byte-identical across calls and is the correct `cache_control` target. Sonnet 5's minimum cacheable prefix is **1024 tokens**; below that, caching silently does nothing and reports `cache_creation_input_tokens: 0` with no error. The taxonomy prompt will land near that threshold, so implementation must verify `usage.cache_read_input_tokens` on a real call rather than assume the marker took effect.

---

## 6. HTTP API

```
GET    /api/ideas?q=&stage=&difficulty=&duration=&treatment=&archived=&sort=&order=
GET    /api/ideas/:id
POST   /api/ideas                     {raw_text, source}
PATCH  /api/ideas/:id                 partial; sets field_overrides for metadata fields
POST   /api/ideas/:id/archive
POST   /api/ideas/:id/restore
DELETE /api/ideas/:id
POST   /api/ideas/:id/notes           {body}
DELETE /api/notes/:id
POST   /api/ideas/:id/links           {other_id}
DELETE /api/ideas/:id/links/:other_id
POST   /api/ideas/:id/merge           {duplicate_id}
POST   /api/ideas/:id/reenrich
GET    /events                        SSE stream
```

`sort` accepts `created_at` (default), `updated_at`, `title`, `difficulty`, or `duration`; `order` accepts `asc` or `desc` (default `desc` for timestamps, `asc` otherwise). `difficulty` and `duration` sort by their semantic order — easy→moderate→insane, quick→average→multi_day — not alphabetically. When `q` is present, results are ordered by FTS rank and `sort` is ignored.

`archived` accepts `false` (default), `true`, or `all`. Merged tombstones are excluded from all list results and resolve to their primary on direct fetch.

SSE event types: `idea.created`, `idea.updated`, `idea.deleted`. The hub is a map of subscriber channels with a mutex; one user, so no further machinery is warranted.

---

## 7. Telegram

**Long polling (`getUpdates`), not webhooks.** Long polling is an outbound call, so the bot runs from a laptop behind NAT with no public URL, no tunnel, and no TLS certificate. This is what makes "local only for v1.0" and "Telegram integration" compatible at all.

Access control is a single `TELEGRAM_ALLOWED_CHAT_ID`; messages from any other chat are dropped and logged.

### Capture

Any plain text message is an idea. A voice note is downloaded via `getFile`, transcribed through whisper.cpp, and the transcript becomes `raw_text` with `source='telegram_voice'` plus a row in `transcripts` retaining the audio path. Either path replies immediately:

> ✓ Saved. *Reading it now…*

The same message is then edited in place via `editMessageText` when enrichment completes:

> ✓ **Crispy chili eggs with scallion oil**
> Easy · Quick · Elevated · Chinese-inspired
> `[Open]`

If enrichment fails, the edit shows the error and a `[Retry]` button.

Editing rather than sending a second message mirrors the SSE behavior in the web app — same idea, same row, filling in — so the two clients read as one system. It also keeps chat history at one message per idea, which matters when the chat is the phone-side view of the backlog.

### Search

`/s <query>` runs FTS5 and replies with an inline keyboard: one button per hit, up to five, labeled with title and stage. `callback_data` carries the idea's UUID, so tapping a result opens that idea's card with its own action buttons. Deep links (`[Open]`) point at `http://localhost:<port>/ideas/<id>`.

No ID is ever displayed, copied, or pasted. This is the specific defect being corrected from iteration 2.

### Command surface (complete for this milestone)

| Input | Behavior |
|---|---|
| plain text | Capture as idea |
| voice note | Transcribe, then capture |
| `/s <query>` | Ranked search, tappable results |
| `/recent` | Ten most recent active ideas, tappable |
| `/help` | Command list |
| callback: open | Show idea card with its deep link |
| callback: retry | Re-run a failed enrichment |

Creation and search are the only Telegram capabilities in this phase, per the original brief. Archiving, editing, merging, and linking are deliberately web-only — `retry` is included because it belongs to making capture reliable rather than being a new capability, and `/recent` is a search convenience over the same index.

---

## 8. Frontend

### Retained

`styles.css`, refactored so all hardcoded colors become custom properties on `:root`. Layout rules, grid definitions, and all four breakpoints are otherwise untouched.

```css
:root {
  --bg: #f6f3ec;  --surface: #fcfaf5;  --border: #dbd4c6;
  --text: #24221d; --muted: #736c62;
  --accent: #4f6b4a; --accent-soft: #eaeee4; --on-accent: #fbfdf8;
  --font-display: Archivo, ui-sans-serif, system-ui, sans-serif;
  --font-ui: Inter, ui-sans-serif, system-ui, sans-serif;
}
```

Components retained as scaffold and rewired to the API: `Sidebar`, `IdeasWorkspace`, `IdeaRow`, `IdeaInspector`, `FilterBar`, `CaptureComposer`.

### Added

- **`react-router`** with `/ideas` and `/ideas/:id`, so Telegram's `[Open]` deep link resolves.
- **A `useIdeas()` hook** owning fetch, optimistic insert, and SSE subscription with reconnect — replacing the fifteen-prop drill through `App.jsx`.
- **Metadata correction UI** in the inspector: each inferred field becomes an editable control; changing one issues a `PATCH` and marks the field in `field_overrides`.
- **Enrichment state on the row**: pending shows a subtle placeholder in the metadata cells; failed shows the error text and a Retry button.

### Removed

`App.jsx` state, `lib/pipeline.js`, `data/ideas.js`, `tests/pipeline.test.js`, `RecipeWorkspace.jsx`, `assets/recipe-thumbnails.png`.

`RecipeWorkspace` is removed rather than retained because this milestone stops before recipes; it returns in a later milestone built against a real schema instead of being kept alive as a mock.

The thumbnail column loses its fabricated food photography. It is replaced with a deterministic mark — a letter drawn from the primary ingredient over a hue hashed from that same value — preserving the row's visual rhythm without implying a photograph exists. Real cover frames arrive in v1.5 when there is media to show.

### Not doing

**No TypeScript migration.** It is a larger diff over code whose value is its CSS, and it is not part of the request. Worth revisiting when the recipe schema lands, where types earn their keep.

---

## 9. Testing

| Layer | Approach |
|---|---|
| `ideas` domain | Table tests. Specifically: archive→restore preserves `stage`; merge sets `merged_into_id` and unions notes, tags, and links; link is symmetric; self-link is rejected. |
| `store` | Real SQLite in a temp file (not `:memory:` — FTS5 triggers must survive a reopen), migrations applied fresh per test. Includes a migrations-run-clean-on-empty-db test. |
| `enrich` | **Recorded fixtures, not mocks.** Real Sonnet 5 responses captured once and replayed offline. One fixture per failure mode: 401, 429, malformed JSON, and schema-valid-but-wrong-enum. |
| `config` | Each missing prerequisite produces a distinct, named startup error. |
| `transcribe` | One committed 3-second OGG through the real binary; skipped with an explicit message when whisper is absent. |
| End-to-end | One Playwright spec: type an idea → row appears immediately → metadata fills in via SSE → search finds it → archive → restore. |

Recorded fixtures rather than SDK mocks are what keep taxonomy logic covered when the API key is dead or the machine is offline — the exact condition that made the prior production incident hard to diagnose.

---

## 10. Configuration

`.env.example`, all values validated at startup:

```
ANTHROPIC_API_KEY=              # or an `ant auth login` profile
SOUSCHEF_MODEL=claude-sonnet-5
SOUSCHEF_EFFORT=low
SOUSCHEF_DB_PATH=./souschef.db
SOUSCHEF_PORT=8420
TELEGRAM_BOT_TOKEN=
TELEGRAM_ALLOWED_CHAT_ID=
WHISPER_BIN=/usr/local/bin/whisper-cli
WHISPER_MODEL=./models/ggml-base.en.bin
AUDIO_DIR=./data/audio
```

No secrets are committed. `.gitignore` covers `node_modules/`, `dist/`, `.playwright-mcp/`, `.superpowers/`, `.env`, `*.db`, `data/`, and `models/`.

---

## 11. Definition of done

1. `make build` produces a single binary; running it serves UI and API on one port.
2. An idea typed in the web app appears on screen in under 50ms and is enriched within roughly 5 seconds without a refresh.
3. The same holds from Telegram, by text and by voice.
4. `/s <query>` in Telegram returns tappable results — no IDs, no copy-paste.
5. List, search, filter (difficulty, duration, treatment, stage, archived), sort, notes, links, merge, archive, restore, and delete all work from the web app.
6. Every inferred field is correctable, and correcting a field protects it from re-enrichment.
7. A dead or unkeyed API key surfaces as a visible error on the row with a working Retry — never as a silent stall.
8. `make test` passes, including the Playwright spec.
9. `PROJECT_LOG.md` records project state, and the branch is merged to `main`.

Work stops at that point for approval before Milestone 2.

---

## 12. Out of scope for Milestone 1

Deferred to later milestones, each getting its own spec: brief generation; recipe generation and the structured review stage; the turn-based revision chat; JSON / Markdown / Schema.org JSON-LD export; script generation; the brand-voice checker; all v1.5 production-checklist features; the Producer Agent; and cloud deployment.

Explicitly out of scope for the product entirely, per the original brief: platform performance data ingestion and performance-to-metadata correlation.
