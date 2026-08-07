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
- **RESOLVED 2026-08-07 — voice notes.** whisper.cpp was never able to transcribe a Telegram voice note. Telegram sends 48kHz Ogg/Opus; whisper.cpp's miniaudio backend cannot decode it, prints `error: failed to read audio file`, and then **exits 0**. `Transcribe` trusted the exit status, so the real explanation was discarded and the user saw "transcript was empty". Fixed three ways: every file is now normalised to 16kHz mono WAV with ffmpeg first; stderr is checked for `error:` lines even on a clean exit; and the malformed `--output-txt false` argument was removed (it is a boolean flag, so the value landed as a bogus input path). ffmpeg is now a validated startup prerequisite. **Confirmed working end to end by Erik on 2026-08-07** — a real Telegram voice note through download → ffmpeg → whisper → saved idea → enrichment.
- **The Milestone 1 audio test fixture was silent** — 85 samples, 0.005s, peak amplitude 0. `say --data-format=LEI16@16000` produced a header with no speech, and Task 14 verified only the file's size and `file` type, never that it contained sound. So even a working pipeline would have produced an empty transcript in tests. Replaced with 3 seconds of real speech in both WAV and Ogg/Opus, and the Opus test asserts on actual transcribed words rather than merely non-empty output.
- **A machine without whisper.cpp cannot run Sous Chef at all** — not merely lose voice capture. `config.Load` (`internal/config/config.go:79-84`) gates *startup* on `WHISPER_BIN` existing and being executable, and on `WHISPER_MODEL` existing, so a fresh checkout following the README on a machine without whisper.cpp installed hits a hard startup failure with no web UI and no Telegram bot. This is the fail-loud-fail-early rule working as designed, but the blast radius is larger than "voice doesn't work" and the README does not say so. Either install whisper.cpp before first run or point `WHISPER_BIN`/`WHISPER_MODEL` at any executable and any regular file to get past the check (that is what the Playwright config does).
- **The Telegram bot is now confirmed against the real API (2026-08-07).** During Milestone 1 no live token was available, so `internal/telegram` shipped verified only by unit tests against an HTTP double and by code inspection. Erik has since exercised it for real: command-menu publication via `setMyCommands`, text capture, `/s` and `/recent` returning tappable results, and voice notes end to end. The wire-format assumptions that were unconfirmed — field names, the `getFile`/download two-step, HTML parse-mode escaping — all hold. What remains unexercised against the real service is the narrower set: the `retry:` callback, the enrichment-failure card, and behaviour when Telegram itself rate-limits or errors. Two of the three must-fix defects found in the final whole-branch review were in this package, which is the honest argument for treating it as unverified rather than merely untested: `enrichAndEdit` spawned untracked goroutines that shutdown never drained, and the bot token leaked into logs and into chat messages via `*url.Error`. Both are fixed, but neither was caught by the package's own tests before review.

### Known residuals — accepted, not blocking merge

Surfaced by the final whole-branch review and its fix-wave re-review. Each was judged non-blocking; recorded here because the execution ledger lives in gitignored `.superpowers/` and does not survive into `main`.

- **Editing an idea reached by deep link, when that idea is not in the loaded list, appears to fail.** `useIdeas.replace()` maps over the loaded list, and an archived or merged idea is not in it, so `useSelectedIdea`'s fetched snapshot is never refreshed. Changing a metadata field on such an idea writes correctly to the server but the control snaps back to blank until reload. The write persists; the UI misreports it. Newly reachable *because* deep links now resolve at all — before the fix that pane was blank. Fix is small: have `replace()` also update the fetched snapshot.
- **`bot.Run`'s goroutine is not joined before `bot.Drain`.** A handler completing `SendMessage` in the instant after context cancellation could call `enrichInBackground` (`wg.Add`) concurrently with Drain's `wg.Wait`. The window is very small and this is strictly better than the pre-fix state (untracked entirely). `httpapi` avoids it because `srv.Shutdown` joins its handlers. A done-channel awaited before `Drain` would close it.
- **One redundant `GET /api/ideas/{id}` on a cold deep-link load**, because the list is empty on first render so the not-in-list check is true before the list resolves. Harmless and self-correcting.
- **`resultsKeyboard` truncates Telegram button labels at a raw byte offset** (`internal/telegram/handlers.go`), the same bug class fixed in `ideas.DeriveTitle`. Impact is cosmetic only — `encoding/json` rewrites a split rune to U+FFFD, so Telegram receives one garbled character rather than an error. Two-line fix.
- **`Merge` and `Correct` are not transactional.** Each performs several writes without a transaction; a failure partway leaves notes duplicated onto the primary with both ideas still live, and re-running duplicates them again. `store` already owns `BeginTx`.
- **`merged_into_id` uses `ON DELETE SET NULL`**, so deleting a primary resurrects every duplicate merged into it as a live idea. Arguably preferable to cascading a delete the user did not ask for, but the behaviour is undecided rather than decided.
- **`POST /api/ideas/{id}/reenrich` is synchronous** with up to 3 × 60s of retries inside the request and no `WriteTimeout` on the server, so a throttled key can hang the browser for ~3 minutes. It also never calls `MarkPending`, so a reload mid-retry shows the stale `failed` state — whereas the Telegram retry path *does* call `MarkPending`. The same user action behaves differently on the two surfaces.
- **`telegram.Deps.Client` is a concrete `*Client`** hardcoded to `api.telegram.org`, so `handleSearch` / `handleCallback` / `resultsKeyboard` cannot be exercised against an HTTP double. This is the largest untested surface in the codebase. Making `Client` an interface is mechanical (7 methods) and unlocks httptest coverage for all of `handlers.go`. Worth doing at the start of Milestone 2.

---

## Log

**2026-08-06** — Brainstormed and approved the Milestone 1 design. Assessed the generated React app for reuse (keep the CSS and component scaffold, discard all data paths). Chose palette and typography with visual mockups. Wrote and committed the spec. Next: implementation plan.

**2026-08-07** — Milestone 1 implementation complete on `feat/milestone-1-capture` (NOT merged to `main` — merge is a separate, explicitly-gated step Erik has not yet approved). Go backend with SQLite + FTS5, Claude Sonnet 5 enrichment, single binary with embedded React and SSE, Telegram capture by text and voice with tappable search (`/s`, `/recent`, inline-keyboard callbacks, no ID ever shown). Added the Playwright end-to-end suite against the built binary.

Facts established this session:
- **Prompt caching engages.** The taxonomy system prompt measures ~1596 tokens, above Sonnet 5's 1024-token minimum. First call wrote 1596 cache-creation tokens; second call read 1596 back (`cache_read_input_tokens` non-zero on the second run).
- **Observed enrichment latency** for a real, non-cache-primed capture: ~4.1s end to end (POST /api/ideas to `enrichment_status = ok`), measured against the live Anthropic API with `SOUSCHEF_MODEL=claude-sonnet-5`.
- **whisper.cpp remains unverified.** Not installed on this machine; the CLI flags in `internal/transcribe/whisper.go` have never run against a real binary, and voice capture is untested end to end. Flagged as a pre-production gap, not a defect — the code path is implemented and unit-tested with a stubbed binary, but the real integration is unverified. See Known issues.
- **Two bugs found and fixed while verifying the definition of done**, both in `web/src`, neither specific to Telegram: (1) `.idea-row` was not the true `:first-child`/`:last-of-type` among its siblings because the list heading and empty/loading states shared its parent — wrapped the rows in their own container (`IdeasWorkspace.jsx`). (2) Archiving/restoring an idea only patched it in place in the client's local state, so it stayed visible in the wrong filtered view until an unrelated event caused a refetch — `useIdeas.js`'s `archive`/`restore` now refetch the current filtered list from the server instead of patching in place.

**2026-08-07 (later)** — Whole-branch review of `feat/milestone-1-capture`, followed by one fix wave. Every task had passed its own review; the whole-branch pass found six defects that per-task review structurally could not see, each of them a decision already made correctly for the web path and never carried across to Telegram or to the browser's cross-origin behaviour. All six fixed on the branch, still unmerged:

- **Telegram enrichment bypassed the shutdown drain.** `enrichAndEdit` was spawned as a bare goroutine from three call sites; `main.run()` drained only the web path. A capture in flight at Ctrl-C wrote to a closed store and left the idea permanently `pending` — with no in-chat recovery, because the Telegram card offers Retry only for `failed`. `Bot` now has the same `Drain(ctx)` as `httpapi.Server`.
- **The bot token leaked into logs and into the chat.** Every Bot API URL embeds the token, and on a transport failure `*url.Error` prints the URL verbatim — reaching the `getUpdates` log line (which fires on any network blip) and, worse, a Telegram message, persisting the token into chat history. All client errors are now redacted.
- **No CSRF or Host defence on the unauthenticated localhost API.** A cross-origin POST with `Content-Type: text/plain` is a CORS simple request with no preflight, so any visited page could create ideas (each firing a paid Claude call), and an unvalidated `Host` let DNS rebinding read the whole backlog and the `/events` stream. Both verified live before the fix. The no-auth decision itself is unchanged and still correct — this closes the gap between "anyone with local shell access can read it" (accepted) and "any website can write to it" (never intended).
- `enrichment_model` was never written (both call sites passed `""`), so no enriched idea had model attribution. Unrecoverable retroactively.
- A merged tombstone did not resolve to its primary on direct fetch — half of the spec §6 promise, and the entire reason tombstones exist instead of deletes.
- Deep links to archived, merged, or off-page ideas rendered a blank pane; `web/src/lib/api.js` had no `getIdea` at all.

Verification after the wave: `go test ./... -race` green, `go vet` clean, `make build` succeeds, Playwright 7/7 including the live-credential SSE spec and two new deep-link specs. The cross-origin POST and rebound-Host GET were re-exercised with curl against the running binary and both return 403 while normal traffic is unaffected.

**2026-08-07 (later)** — Milestone 1 merged to `main` and pushed. Three defects surfaced only through real first-hand use, none of which the automated suite could have caught, because each hid behind an assumption the tests themselves supplied:

- `.env` was never read. `config.Load` only called `os.Getenv`, and every verification during development passed configuration inline, so the documented setup path was never once executed. Fixed with godotenv; ambient variables still take precedence.
- Voice notes never worked. Telegram sends Ogg/Opus, whisper.cpp cannot decode it and **exits 0 anyway**, so the real error was discarded and the user saw "transcript was empty". Fixed with an ffmpeg conversion step plus a stderr check that does not trust the exit code.
- The audio test fixture was silent (peak amplitude 0), so three integration tests looked meaningful while being incapable of passing. A test asset nobody checks for *content* converts an absence of coverage into a false impression of it.

Milestone 1 is now confirmed working on every path: web and Telegram capture, metadata enrichment, Telegram commands and search, and voice notes end to end.

Next: Milestone 2 (brief generation), which does not start until Erik asks for it.
