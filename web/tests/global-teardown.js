import fs from 'node:fs';

// Remove the per-run scratch database so /tmp does not fill with them. The
// paths come from the env the config handed to the web server, so teardown
// cannot drift from whatever setup actually used.
export default function globalTeardown() {
  const db = process.env.SOUSCHEF_PW_DB;
  const audio = process.env.SOUSCHEF_PW_AUDIO;
  if (db) {
    for (const f of [db, `${db}-wal`, `${db}-shm`]) fs.rmSync(f, { force: true });
  }
  if (audio) fs.rmSync(audio, { recursive: true, force: true });
}
