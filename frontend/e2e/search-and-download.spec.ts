import { test, expect } from '@playwright/test';
import { selectWorkload } from './helpers.js';

test.describe('Server-side search', () => {
  test('switching to Server mode hides the page-search field and shows search controls', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await expect(page.getByPlaceholder('Search…')).toBeVisible();

    await page.getByRole('button', { name: 'Server', exact: true }).click();
    await expect(page.getByPlaceholder('Search…')).not.toBeVisible();
    await expect(page.getByPlaceholder('Search server-side…')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
  });

  test('running a search streams and highlights matching results', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await page.getByRole('button', { name: 'Server', exact: true }).click();

    await page.getByPlaceholder('Search server-side…').fill('burst 5');
    await page.getByRole('button', { name: 'Search' }).click();

    const mark = page.locator('mark', { hasText: 'burst 5' }).first();
    await expect(mark).toBeVisible();
  });

  test('jumping to context switches back to page mode filtered to that pod', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');
    await page.getByRole('button', { name: 'Server', exact: true }).click();

    await page.getByPlaceholder('Search server-side…').fill('burst 5');
    await page.getByRole('button', { name: 'Search' }).click();

    const resultRow = page.locator('mark', { hasText: 'burst 5' }).first();
    await expect(resultRow).toBeVisible();
    await resultRow.click();

    // Back in page mode, with the log toolbar's page-search field visible again.
    await expect(page.getByPlaceholder('Search…')).toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });
});

test.describe('Log download', () => {
  test('the download button triggers a file download scoped to the current workload', async ({ page }) => {
    await selectWorkload(page, 'default', 'Deployment', 'web-app');

    const downloadPromise = page.context().waitForEvent('download');
    await page.getByLabel('Download logs').click();
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toContain('default_Deployment-web-app');
  });
});
