import { test, expect } from '@playwright/test';

test.describe('PodSidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows namespaces from the API', async ({ page }) => {
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).toBeVisible();
  });

  test('expands a namespace to show its deployments (default view)', async ({ page }) => {
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();
    await expect(page.getByText('api-server')).toBeVisible();
  });

  test('collapses the namespace list on a second click', async ({ page }) => {
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();

    await page.getByText('default').click();
    await expect(page.getByText('web-app')).not.toBeVisible();
  });

  test('can switch to pods view mode and see individual pods', async ({ page }) => {
    // The view mode selector is a <Select> whose visible value is "Deployments".
    await page.getByRole('combobox').click();
    await page.getByRole('option', { name: 'Pods' }).click();

    await page.getByText('default').click();
    await expect(page.getByText('web-app-6d8c7f')).toBeVisible();
    await expect(page.getByText('api-server-5b4c9e')).toBeVisible();
  });

  test('selecting a deployment shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // LogToolbar renders the deployment name in a chip (sidebar uses ListItemText, not a chip)
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });

  test('selecting a pod shows the log panel toolbar header', async ({ page }) => {
    await page.getByRole('combobox').click();
    await page.getByRole('option', { name: 'Pods' }).click();

    await page.getByText('default').click();
    await page.getByText('web-app-6d8c7f').click();

    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app-6d8c7f' })).toBeVisible();
  });
});
