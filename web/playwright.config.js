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
    // config.Load() refuses to start without these. A scratch DB path under
    // /tmp keeps the test run from ever touching a real souschef.db, and the
    // whisper values only need to point at *some* executable file/regular
    // file — no voice test exercises transcription here. ANTHROPIC_API_KEY
    // is inherited from process.env (via the spread) rather than set here,
    // so the live-credential spec runs for real when one is present in the
    // environment and skips cleanly when it isn't.
    env: {
      ...process.env,
      SOUSCHEF_PORT: '8420',
      SOUSCHEF_DB_PATH: '/tmp/souschef-playwright.db',
      TELEGRAM_BOT_TOKEN: 'test-token-not-real',
      TELEGRAM_ALLOWED_CHAT_ID: '1',
      WHISPER_BIN: '/bin/sh',
      WHISPER_MODEL: '/etc/hosts',
      AUDIO_DIR: '/tmp/souschef-playwright-audio',
    },
  },
});
