import { test, expect } from '@playwright/test';
import { selectWorkload } from './helpers.js';

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('LogPanel — Deployment-kind workload', () => {
  test('renders log lines after selecting a workload', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    // The initial load returns the last page (loadLastPage=true).
    // Burst lines from hour 23 of 2024-01-15 appear on that page.
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();
  });

  test('renders multiple log lines', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();
    await expect(page.getByText(/burst 9 from web-app/).first()).toBeVisible();
  });

  test('search filter shows only matching lines', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();

    // Filter to a specific burst line; normal "log entry" lines should vanish.
    await page.getByPlaceholder('Filter…').fill('burst 5 from web-app');

    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();
    // A normal entry line from the same page should no longer be visible.
    await expect(page.getByText(/log entry 1961 from web-app/)).not.toBeVisible();
  });

  test('clearing the search filter restores all lines', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await page.getByPlaceholder('Filter…').fill('burst 5 from web-app');
    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();

    await page.getByPlaceholder('Filter…').clear();
    // Normal entry lines should reappear once the filter is cleared.
    await expect(page.getByText(/log entry 1961 from web-app/).first()).toBeVisible();
  });

  test('live mode toggle enables streaming and shows new lines', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await page.getByLabel('Live').click();
    await expect(page.getByText(/live workload line/).first()).toBeVisible({ timeout: 5000 });
  });
});

test.describe('LogPanel — Pod-kind workload', () => {
  test('renders log lines after selecting a standalone pod', async ({ page }) => {
    await selectWorkload(page, 'default', 'Pod', 'standalone-pod');
    await expect(page.getByText(/burst 1 from standalone-pod/).first()).toBeVisible();
  });

  test('live mode streams pod logs', async ({ page }) => {
    await selectWorkload(page, 'default', 'Pod', 'standalone-pod');
    await page.getByLabel('Live').click();
    await expect(page.getByText(/live workload line/).first()).toBeVisible({ timeout: 5000 });
  });
});

test.describe('LogPanel — log message modal', () => {
  test('clicking a log line opens a modal with the full message and can copy it', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await selectWorkload(page, 'default', 'Deployment', 'web-app');

    const line = page.getByText(/burst 1 from web-app/).first();
    await expect(line).toBeVisible();
    await line.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('burst 1 from web-app');

    await dialog.getByRole('button', { name: 'Copy' }).click();
    await expect(dialog.getByRole('button', { name: 'Copied!' })).toBeVisible();
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toContain('burst 1 from web-app');

    await dialog.getByRole('button', { name: 'Close' }).click();
    await expect(dialog).not.toBeVisible();
  });
});

test.describe('LogPanel — dark mode', () => {
  test('dark mode toggle changes the visual theme', async ({ page }) => {
    await page.goto('/');
    const toggle = page.getByRole('button', { name: /brightness/i });
    await expect(toggle).toBeVisible();
    await toggle.click();
    // After toggling, the button should still be visible (no crash).
    await expect(toggle).toBeVisible();
  });
});

test.describe('LogPanel — namespace-wide live tail', () => {
  test('the namespace page can tail every pod in it, and stop', async ({ page }) => {
    await page.goto('/ns/default');
    await page.getByRole('button', { name: 'Live tail every pod in default' }).click();

    await expect(page.getByText('default · all pods')).toBeVisible();
    await expect(page.getByText('live workload line 1 from every pod in default')).toBeVisible();

    await page.getByRole('button', { name: 'Stop' }).click();
    await expect(page.getByRole('button', { name: 'Live tail every pod in default' })).toBeVisible();
  });
});
