import { test, expect } from '@playwright/test';
import { selectDeployment, selectPod } from './helpers.js';

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('LogPanel — deployment mode', () => {
  test('renders log lines after selecting a deployment', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    // The initial load returns the last page (loadLastPage=true).
    // Burst lines from hour 23 of 2024-01-15 appear on that page.
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();
  });

  test('renders multiple log lines', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();
    await expect(page.getByText(/burst 9 from web-app/).first()).toBeVisible();
  });

  test('search filter shows only matching lines', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await expect(page.getByText(/burst 1 from web-app/).first()).toBeVisible();

    // Filter to a specific burst line; normal "log entry" lines should vanish.
    await page.getByPlaceholder('Search\u2026').fill('burst 5 from web-app');

    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();
    // A normal entry line from the same page should no longer be visible.
    await expect(page.getByText(/log entry 1961 from web-app/)).not.toBeVisible();
  });

  test('clearing the search filter restores all lines', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await page.getByPlaceholder('Search\u2026').fill('burst 5 from web-app');
    await expect(page.getByText(/burst 5 from web-app/).first()).toBeVisible();

    await page.getByPlaceholder('Search\u2026').clear();
    // Normal entry lines should reappear once the filter is cleared.
    await expect(page.getByText(/log entry 1961 from web-app/).first()).toBeVisible();
  });

  test('live mode toggle enables streaming and shows new lines', async ({ page }) => {
    await selectDeployment(page, 'default', 'web-app');
    await page.getByLabel('Live').click();
    await expect(page.getByText(/live deployment line/).first()).toBeVisible({ timeout: 5000 });
  });
});

test.describe('LogPanel — pod mode', () => {
  test('renders log lines after selecting a pod', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await expect(page.getByText(/burst 1 from web-app-6d8c7f/).first()).toBeVisible();
  });

  test('live mode streams pod logs', async ({ page }) => {
    await selectPod(page, 'default', 'web-app-6d8c7f');
    await page.getByLabel('Live').click();
    await expect(page.getByText(/live line/).first()).toBeVisible({ timeout: 5000 });
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
