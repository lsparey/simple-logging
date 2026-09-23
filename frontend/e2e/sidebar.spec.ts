import { test, expect } from '@playwright/test';

test.describe('PodSidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows the section menu with no namespaces until a section is chosen', async ({ page }) => {
    await expect(page.getByText('Workloads')).toBeVisible();
    await expect(page.getByText('Indexes')).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('shows namespaces from the API after choosing Workloads', async ({ page }) => {
    await page.getByText('Workloads').click();
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).toBeVisible();
  });

  test('expands a namespace to show its workloads, tagged by kind', async ({ page }) => {
    await page.getByText('Workloads').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();
    await expect(page.getByText('api-server')).toBeVisible();
    await expect(page.getByText('standalone-pod')).toBeVisible();
    await expect(page.getByText('Deploy').first()).toBeVisible();
    await expect(page.getByText('Pod').first()).toBeVisible();
  });

  test('collapses the namespace list on a second click', async ({ page }) => {
    await page.getByText('Workloads').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();

    await page.getByText('default').click();
    await expect(page.getByText('web-app')).not.toBeVisible();
  });

  test('the back arrow returns to the section menu', async ({ page }) => {
    await page.getByText('Workloads').click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByRole('button', { name: 'Back' }).click();
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.getByText('Workloads')).toBeVisible();
  });

  test('selecting a Deployment-kind workload shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Workloads').click();
    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // LogToolbar renders the workload name in a chip (sidebar uses ListItemText, not a chip)
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });

  test('selecting a Pod-kind workload shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Workloads').click();
    await page.getByText('default').click();
    await page.getByText('standalone-pod').click();

    await expect(page.locator('.MuiChip-root').filter({ hasText: 'standalone-pod' })).toBeVisible();
  });
});

test.describe('MobileSidebarNav', () => {
  test('the bottom icons are visible and no namespaces show until one is tapped', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Indexes' })).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('tapping an icon pops up the namespace list, and selecting collapses it back to the icons', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await page.getByRole('button', { name: 'Workloads' }).click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // The overlay auto-closes back down to just the bottom icons after a selection.
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
  });
});
