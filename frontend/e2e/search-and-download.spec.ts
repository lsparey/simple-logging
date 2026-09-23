import { test, expect } from '@playwright/test';
import { selectWorkload } from './helpers.js';

test.describe('Server-side search', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Open server-side search' }).click();
  });

  test('opens as a standalone page with scope and query controls', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Search' })).toBeVisible();
    await expect(page.getByLabel('Namespace')).toBeVisible();
    await expect(page.getByLabel('Workload kind')).toBeVisible();
    await expect(page.getByLabel('Workload name')).toBeVisible();
    await expect(page.getByPlaceholder('Query…')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Search', exact: true })).toBeVisible();
  });

  test('"All" is the top option for namespace and workload kind', async ({ page }) => {
    await page.getByLabel('Namespace').click();
    const namespaceOptions = page.getByRole('listbox').getByRole('option');
    await expect(namespaceOptions.first()).toHaveText('All');
    await page.keyboard.press('Escape');

    await page.getByLabel('Workload kind').click();
    const kindOptions = page.getByRole('listbox').getByRole('option');
    await expect(kindOptions.first()).toHaveText('All');
    await page.keyboard.press('Escape');
  });

  test('running a scoped search streams and highlights matching results', async ({ page }) => {
    await page.getByLabel('Namespace').click();
    await page.getByRole('option', { name: 'default' }).click();
    await page.getByLabel('Workload kind').click();
    await page.getByRole('option', { name: 'Deployments' }).click();
    await page.getByLabel('Workload name').fill('web-app');
    await page.getByPlaceholder('Query…').fill('burst 5');
    await page.getByRole('button', { name: 'Search', exact: true }).click();

    const mark = page.locator('mark', { hasText: 'burst 5' }).first();
    await expect(mark).toBeVisible();
  });

  test('the workload name field suggests names for the selected namespace and kind', async ({ page }) => {
    await page.getByLabel('Namespace').click();
    await page.getByRole('option', { name: 'default' }).click();
    await page.getByLabel('Workload kind').click();
    await page.getByRole('option', { name: 'Deployments' }).click();

    await page.getByLabel('Workload name').click();
    await expect(page.getByRole('option', { name: 'web-app' })).toBeVisible();
    await expect(page.getByRole('option', { name: 'api-server' })).toBeVisible();
  });

  test('jumping to context navigates to the hit pod, scoped by kind Pod', async ({ page }) => {
    await page.getByLabel('Namespace').click();
    await page.getByRole('option', { name: 'default' }).click();
    await page.getByLabel('Workload kind').click();
    await page.getByRole('option', { name: 'Deployments' }).click();
    await page.getByLabel('Workload name').fill('web-app');
    await page.getByPlaceholder('Query…').fill('burst 5');
    await page.getByRole('button', { name: 'Search', exact: true }).click();

    const resultRow = page.locator('mark', { hasText: 'burst 5' }).first();
    await expect(resultRow).toBeVisible();
    await resultRow.click();

    await expect(page).toHaveURL(/\/ns\/default\/Pod\//);
    // Wait for the search page itself to unmount before checking for the log
    // toolbar's chip: while both are momentarily present (React hasn't
    // re-rendered yet even though the URL already changed), a locator
    // matching the shared MuiChip-root class picks up the many still-mounted
    // search result chips too, which fails as a strict-mode violation rather
    // than something toBeVisible()'s usual retrying can wait out.
    await expect(page.getByRole('heading', { name: 'Search' })).not.toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
    // Back on the regular log page (the client-side filter field, not the search page's query field).
    await expect(page.getByPlaceholder('Filter…')).toBeVisible();
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
