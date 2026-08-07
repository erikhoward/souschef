# PROJECT_LOG — Sous Chef

Breadcrumb trail for project state, version, and context. Read this before starting work.

---

## Current state

**Version:** 1.0.0-rc1 (Milestone 1 code-complete, NOT yet merged)
**Phase:** Milestone 1 fully implemented and verified on `feat/milestone-1-capture`; awaiting Erik's review and approval to merge into `main`
**Branch:** `feat/milestone-1-capture` (not yet merged — `main` is still at the pre-implementation commit)

**What exists right now (on the feature branch, unmerged):**
- Go backend (`cmd/souschef`, `internal/...`): SQLite + FTS5 storage, `internal/ideas` CRUD/search/merge/archive service, Claude Sonnet 5 enrichment (`internal/enrich`), whisper.cpp transcription wrapper (`internal/transcribe`), REST + SSE API (`internal/httpapi`), single-binary embedded-React serving with SPA fallback, graceful shutdown with an enrichment drain.
- Telegram bot (`internal/telegram`): text and voice capture, edit-in-place enrichment, `/s <query>` and `/recent` returning tappable inline-keyboard results (no ID ever shown in visible text), callback routing (open/retry), app-managed command menu (no BotFather configuration needed).
- `web/`: retheme (Slate & Sage palette, self-hosted Archivo + Inter), router, live SSE-backed ideas list, enrichment states, retry control, inline metadata correction with override protection.
- Playwright end-to-end suite (`web/tests/capture.spec.js`) running against the built binary.

**What does not exist yet:** everything past Milestone 1 (brief generation, recipe generation, export, script generation). The merge to `main` itself has not happened — see Log below.

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
| 1 | Capture & organize: Go backend, SQLite, Claude inference, full ideas CRUD, retheme, Telegram text + voice capture and search | **Code-complete, verified, awaiting review & merge** |
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
- **Prompt caching threshold.** Sonnet 5's minimum cacheable prefix is 1024 tokens; below that, `cache_control` silently does nothing. Verified against a real call (see Log entry below): the taxonomy system prompt measures ~1596 tokens, above the minimum, and caching does engage.
- **whisper.cpp is a documented prerequisite**, not a bundled dependency. Startup validation must give a clear, actionable message when it's missing.
- **whisper.cpp is not installed on the development/CI machine used to build Milestone 1.** The CLI flags in `internal/transcribe/whisper.go` (`--model`, `--file`, `--output-txt false`, `--no-prints`) are unverified against a real binary, and voice capture (Telegram voice note → transcript → saved idea) has never been exercised end-to-end. This must be verified against a real whisper.cpp install (Homebrew build and/or a source build — the two are known to differ on flag names) before voice capture can be trusted in production.

---

## Log

**2026-08-06** — Brainstormed and approved the Milestone 1 design. Assessed the generated React app for reuse (keep the CSS and component scaffold, discard all data paths). Chose palette and typography with visual mockups. Wrote and committed the spec. Next: implementation plan.

**2026-08-07** — Milestone 1 implementation complete on `feat/milestone-1-capture` (NOT merged to `main` — merge is a separate, explicitly-gated step Erik has not yet approved). Go backend with SQLite + FTS5, Claude Sonnet 5 enrichment, single binary with embedded React and SSE, Telegram capture by text and voice with tappable search (`/s`, `/recent`, inline-keyboard callbacks, no ID ever shown). Added the Playwright end-to-end suite against the built binary.

Facts established this session:
- **Prompt caching engages.** The taxonomy system prompt measures ~1596 tokens, above Sonnet 5's 1024-token minimum. First call wrote 1596 cache-creation tokens; second call read 1596 back (`cache_read_input_tokens` non-zero on the second run).
- **Observed enrichment latency** for a real, non-cache-primed capture: ~4.1s end to end (POST /api/ideas to `enrichment_status = ok`), measured against the live Anthropic API with `SOUSCHEF_MODEL=claude-sonnet-5`.
- **whisper.cpp remains unverified.** Not installed on this machine; the CLI flags in `internal/transcribe/whisper.go` have never run against a real binary, and voice capture is untested end to end. Flagged as a pre-production gap, not a defect — the code path is implemented and unit-tested with a stubbed binary, but the real integration is unverified. See Known issues.
- **Two bugs found and fixed while verifying the definition of done**, both in `web/src`, neither specific to Telegram: (1) `.idea-row` was not the true `:first-child`/`:last-of-type` among its siblings because the list heading and empty/loading states shared its parent — wrapped the rows in their own container (`IdeasWorkspace.jsx`). (2) Archiving/restoring an idea only patched it in place in the client's local state, so it stayed visible in the wrong filtered view until an unrelated event caused a refetch — `useIdeas.js`'s `archive`/`restore` now refetch the current filtered list from the server instead of patching in place.

Next: Erik reviews the branch and approves (or requests changes to) the merge. Milestone 2 (brief generation) does not start until that happens.
