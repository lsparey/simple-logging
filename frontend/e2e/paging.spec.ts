/**
 * End-to-end paging tests.
 *
 * Fixture layout (2160 lines per source, 3 days × 24 hours × 30 lines):
 *   Day 1 (2024-01-13): indices   0 –  719  ← never on initial load
 *   Day 2 (2024-01-14): indices 720 – 1439  ← never on initial load
 *   Day 3 (2024-01-15): indices 1440 – 2159 ← initial load returns 1960–2159
 *
 * loadLastPage=true returns the last 200 lines (indices 1960–2159), all from
 * 2024-01-15. Known landmarks used in assertions below:
 *
 *   First entry on last page (index 1960):
 *     "2024-01-15T17:30:30Z INFO log entry 1961 from <source>"
 *
 *   Last burst block (hour 23, indices 2150–2159):
 *     "2024-01-15T23:00:00Z INFO burst 1..10 from <source>"
 *
 *   Very first line (index 0, pages 9+ back from initial load):
 *     "2024-01-13T00:00:00Z INFO log entry 1 from <source>"
 *
 * Page chip (bottom-right of log list):
 *   Initial load: "… · Page 1 / 1"  (200 lines → 1 page)
 *   After one load-older: "… · Page 1 / 2"  (400 lines → 2 pages; scroll
 *     position kept, so visible start ≈ index 200 → reversedPageNum = 1)
 */
import { test, expect } from '@playwright/test';
import { selectDeployment, selectPod, scrollLogListToTop } from './helpers.js';

// ---------------------------------------------------------------------------
// Deployment view
// ---------------------------------------------------------------------------

test.describe('Paging — deployment view', () => {
  test('initial load shows the most recent logs (2024-01-15)', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    // All visible lines should come from the last page (2024-01-15).
    await expect(page.getByText(/2024-01-15T.*from web-app/).first()).toBeVisible();
  });

  test('initial load does NOT show the oldest logs (2024-01-13)', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    // Wait for the page to settle before asserting absence.
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();
    // Line 1 from day 1 is well outside the last 200 lines.
    await expect(page.getByText('log entry 1 from web-app')).not.toBeVisible();
  });

  test('page chip shows "Page 1 / 1" on initial load', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();
    // 200 lines loaded → totalPages = 1.
    await expect(page.getByText(/Page 1 \/ 1/)).toBeVisible();
  });

  test('"scroll for older" indicator is shown when older pages exist', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();
    // prevPageToken is set → hasOlderLogs → chip appears.
    await expect(page.getByText('↑ Scroll for older')).toBeVisible();
  });

  test('loading older updates the page chip and preserves latest lines', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    // Wait for initial load and the settling period (LogList suppresses near-top
    // triggers for 150 ms after the initial scroll-to-bottom).
    await expect(page.getByText('↑ Scroll for older')).toBeVisible();
    await page.waitForTimeout(300);

    // Scroll the react-window container to the top to trigger loadOlder.
    await scrollLogListToTop(page);

    // After prepending 200 more lines (400 total → 2 pages), the scroll
    // position is adjusted to keep previously visible rows in view, placing
    // the viewport at approximately index 200 → Page 1 / 2 (reversed).
    await expect(page.getByText(/Page 1 \/ 2/)).toBeVisible({ timeout: 5000 });

    // The most-recent lines should still be reachable (still in the store).
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible({
      timeout: 5000,
    });
  });

  test('"scroll for older" chip remains visible after loading one older page', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await expect(page.getByText('↑ Scroll for older')).toBeVisible();
    await page.waitForTimeout(300);
    await scrollLogListToTop(page);
    await expect(page.getByText(/Page 1 \/ 2/)).toBeVisible({ timeout: 5000 });

    // 1760 lines are still older than what we've loaded → chip still visible.
    await expect(page.getByText('↑ Scroll for older')).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// Pod view
// ---------------------------------------------------------------------------

test.describe('Paging — pod view', () => {
  test('initial load shows the most recent logs (2024-01-15)', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await expect(page.getByText(/2024-01-15T.*from web-app-6d8c7f/).first()).toBeVisible();
  });

  test('initial load does NOT show the oldest logs (2024-01-13)', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await expect(page.getByText(/burst 1 from web-app-6d8c7f/).first()).toBeVisible();
    await expect(page.getByText('log entry 1 from web-app-6d8c7f')).not.toBeVisible();
  });

  test('"scroll for older" indicator is shown when older pages exist', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await expect(page.getByText(/burst 1 from web-app-6d8c7f/).first()).toBeVisible();
    await expect(page.getByText('↑ Scroll for older')).toBeVisible();
  });

  test('loading older updates the page chip', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await expect(page.getByText('↑ Scroll for older')).toBeVisible();
    await page.waitForTimeout(300);

    await scrollLogListToTop(page);

    await expect(page.getByText(/Page 1 \/ 2/)).toBeVisible({ timeout: 5000 });
  });
});
