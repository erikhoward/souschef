import { defineConfig } from '@playwright/test';
import os from 'node:os';
import path from 'node:path';

// A fresh database per run, decided HERE rather than in globalSetup.
//
// The scratch path used to be a fixed name that nothing ever cleaned, so it
// accumulated across runs — after a handful it held 31 ideas including
// stranded `pending` rows from suites killed mid-enrichment, and specs that
// assert on counts get less trustworthy as that pile grows. Deleting the file
// in globalSetup does NOT work: Playwright starts the web server before
// globalSetup runs, so the delete pulls the file out from under a live writer
// and the server keeps using the orphaned inode. Choosing a unique name at
// config load happens before anything starts, so there is no race.
const runDB = path.join(os.tmpdir(), `souschef-pw-${Date.now()}.db`);
const runAudio = path.join(os.tmpdir(), `souschef-pw-audio-${Date.now()}`);

// Publish them on this process too — globalTeardown runs here, not in the web
// server, so it cannot see the server's env block.
process.env.SOUSCHEF_PW_DB = runDB;
process.env.SOUSCHEF_PW_AUDIO = runAudio;

export default defineConfig({
  testDir: './tests',
  globalTeardown: './tests/global-teardown.js',
  timeout: 30_000,
  use: { baseURL: 'http://127.0.0.1:8420', trace: 'retain-on-failure' },
  // Test the real binary, not the dev server: the embedded-asset path and
  // the SPA fallback only exist in the built artifact.
  webServer: {
    command: '../bin/souschef',
    url: 'http://127.0.0.1:8420',
    // Always start our own server. Reusing one means Playwright silently
    // skips the env block below — so the suite would run against whatever
    // database and configuration that process happened to have, and
    // globalSetup would delete the DB out from under a live writer.
    reuseExistingServer: false,
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
      SOUSCHEF_PW_DB: runDB,
      SOUSCHEF_PW_AUDIO: runAudio,
      SOUSCHEF_PORT: '8420',
      SOUSCHEF_DB_PATH: runDB,
      TELEGRAM_BOT_TOKEN: 'test-token-not-real',
      TELEGRAM_ALLOWED_CHAT_ID: '1',
      WHISPER_BIN: '/bin/sh',
      WHISPER_MODEL: '/etc/hosts',
      // Likewise a stand-in: no spec here exercises transcription, and
      // pointing at a real ffmpeg would make the suite fail on machines that
      // do not have one installed.
      FFMPEG_BIN: '/bin/sh',
      AUDIO_DIR: runAudio,
    },
  },
});
