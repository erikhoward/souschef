# PROJECT_LOG — Sous Chef

Breadcrumb trail for project state, version, and context. Read this before starting work.

---

## Current state

**Version:** 0.1.0 (pre-implementation)
**Phase:** Milestone 1 design approved, implementation plan not yet written
**Branch:** `main`

**What exists right now:**
- A Codex-generated Vite + React app at the repo root — visually good, functionally hollow (seed data, fake inference, no backend). Roughly 25% is being kept; see the disposition table in the Milestone 1 spec.
- An approved design spec: `docs/superpowers/specs/2026-08-06-souschef-capture-design.md`

**What does not exist yet:** the Go backend, SQLite persistence, Claude integration, Telegram bot, whisper.cpp transcription, and the repo restructure that moves the web app into `web/`.

---

## Iteration history

This is the fourth attempt at the product. Each prior iteration failed informatively:

1. **Petal Labs toolchain** — premature; solved problems the product didn't have yet.
2. **Telegram-first** — capture was right (voice, natural language, instant, phone-friendly); retrieval was unusable, because finding an existing idea meant copy-pasting IDs. Editing in Telegram was painful.
3. **Codex-generated React app** — beautiful and non-working. No backend, no agent, no Telegram, and the web project placed at repo root leaving no room for a server.

Validated and carried forward: **rapid voice/text capture is the core value**, and **seeing the backlog with each item's stage at a glance is second**.

---

## Milestones

| # | Scope | Status |
|---|---|---|
| 1 | Capture & organize: Go backend, SQLite, Claude inference, full ideas CRUD, retheme, Telegram text + voice capture and search | **Design approved** — planning next |
| 2 | Brief generation | Not started |
| 3 | Recipe generation + structured review + revision chat | Not started |
| 4 | Export (JSON, Markdown, Schema.org JSON-LD) | Not started |
| 5 | Script generation + brand-voice checker | Not started |
| — | v1.5: production checklists, media linking, Producer Agent | Not started |

Each milestone gets its own spec, plan, and merge. Work pauses for approval at each milestone boundary.

---

## Key decisions (Milestone 1)

| Decision | Choice | Why |
|---|---|---|
| Backend language | Go | CRUD + HTTP + long-poll + LLM calls; single static binary, fast iteration |
| Storage | SQLite, single file | Portability requirement is about export, not storage engine; single user |
| Architecture | One binary, embedded React, SSE push | Instant capture, background enrichment, no ambiguity about job state |
| Transcription | whisper.cpp, local | Claude is text-only; keeps "local only for v1.0" literal, no quota to exhaust |
| Inference model | `claude-sonnet-5`, configurable | "Frontier only" was scoped to recipe generation; inference is easier. Config value, not a constant |
| Palette | Slate & Sage (`#4f6b4a` accent on warm paper) | Promotes a green already present in the stylesheet; replaces the tomato-soup red |
| Typography | Archivo display + Inter UI, self-hosted | Georgia was load-bearing for editorial/chrome separation; a single sans flattens it |
| Web auth | None (localhost, single user) | Telegram gets a chat-ID allowlist, since that surface is exposed |

---

## Known issues / watch items

- **Enrichment failures must never be silent.** A prior production incident surfaced an OpenAI 429 (no-quota key) as an indefinite "waiting for enrichment" hang. Milestone 1 stores `enrichment_status` + verbatim `enrichment_error` and surfaces both on the row with a Retry control. Startup validation refuses to boot on a missing credential, whisper binary, or model file.
- **Prompt caching threshold.** Sonnet 5's minimum cacheable prefix is 1024 tokens; below that, `cache_control` silently does nothing. The taxonomy system prompt will land near that line — verify `usage.cache_read_input_tokens` on a real call rather than assuming.
- **whisper.cpp is a documented prerequisite**, not a bundled dependency. Startup validation must give a clear, actionable message when it's missing.

---

## Log

**2026-08-06** — Brainstormed and approved the Milestone 1 design. Assessed the generated React app for reuse (keep the CSS and component scaffold, discard all data paths). Chose palette and typography with visual mockups. Wrote and committed the spec. Next: implementation plan.
