# Sous Chef

A local-first recipe content pipeline for capturing ideas, shaping them into briefs, reviewing recipes, and moving content toward a short-form video script.

## Run locally

```bash
bun install
bun run dev
```

## Included MVP flow

- Capture ideas in natural language with inferred creation metadata.
- Search, filter, archive, restore, delete, merge, and link related ideas.
- Progress ideas through brief, recipe-review, and script-ready workflow stages.
- Review a structured recipe, record test status, approve it, and open the next script stage.

This local UI uses seeded data and deterministic metadata inference. Telegram capture, persisted storage, AI model calls, and export formats are the next implementation layer.
