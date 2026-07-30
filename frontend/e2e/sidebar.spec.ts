import { test, expect } from '@playwright/test';

test.describe('PodSidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows the section menu with no namespaces until a section is chosen', async ({ page }) => {
    await expect(page.getByText('Pods')).toBeVisible();
    await expect(page.getByText('Deployments')).toBeVisible();
    await expect(page.getByText('Indexes')).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('shows namespaces from the API after choosing Deployments', async ({ page }) => {
    await page.getByText('Deployments').click();
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).toBeVisible();
  });

  test('expands a namespace to show its deployments (default view)', async ({ page }) => {
    await page.getByText('Deployments').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();
    await expect(page.getByText('api-server')).toBeVisible();
  });

  test('collapses the namespace list on a second click', async ({ page }) => {
    await page.getByText('Deployments').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();

    await page.getByText('default').click();
    await expect(page.getByText('web-app')).not.toBeVisible();
  });

  test('the back arrow returns to the section menu', async ({ page }) => {
    await page.getByText('Deployments').click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByRole('button', { name: 'Back' }).click();
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.getByText('Pods')).toBeVisible();
  });

  test('can switch to pods view mode and see individual pods', async ({ page }) => {
    await page.getByText('Pods').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app-6d8c7f')).toBeVisible();
    await expect(page.getByText('api-server-5b4c9e')).toBeVisible();
  });

  test('selecting a deployment shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Deployments').click();
    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // LogToolbar renders the deployment name in a chip (sidebar uses ListItemText, not a chip)
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });

  test('selecting a pod shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Pods').click();
    await page.getByText('default').click();
    await page.getByText('web-app-6d8c7f').click();

    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app-6d8c7f' })).toBeVisible();
  });
});

test.describe('MobileSidebarNav', () => {
  test('the bottom icons are visible and no namespaces show until one is tapped', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Pods' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Deployments' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Indexes' })).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('tapping an icon pops up the namespace list, and selecting collapses it back to the icons', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await page.getByRole('button', { name: 'Deployments' }).click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // The overlay auto-closes back down to just the bottom icons after a selection.
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Deployments' })).toBeVisible();
  });
});
