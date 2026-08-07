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
  // Scoped to .inspector: an unscoped `button:has-text("Archive")` is a
  // substring match that also matches the filter tab labelled "Archived",
  // which sits earlier in the DOM and would eat the click instead.
  await page.click('.inspector button:has-text("Archive")');
  await expect(page.locator('.idea-row', { hasText: unique })).toHaveCount(0);

  await page.click('button:has-text("Archived")');
  await expect(page.locator('.idea-row', { hasText: unique })).toHaveCount(1);

  await page.click('.idea-row:first-child');
  await page.click('.inspector button:has-text("Restore")');
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
