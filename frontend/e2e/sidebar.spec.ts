import { test, expect } from '@playwright/test';

test.describe('PodSidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows the section menu with no namespaces until a section is chosen', async ({ page }) => {
    await expect(page.getByText('Indexes')).toBeVisible();
    await expect(page.getByText('Deployments')).toBeVisible();
    await expect(page.getByText('StatefulSets')).toBeVisible();
    await expect(page.getByText('DaemonSets')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Jobs', exact: true })).toBeVisible();
    await expect(page.getByText('CronJobs')).toBeVisible();
    await expect(page.getByText('Pods')).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('shows namespaces from the API after choosing a workload kind', async ({ page }) => {
    await page.getByText('Deployments').click();
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).toBeVisible();
  });

  test('expanding a namespace under Deployments shows only Deployment-kind workloads', async ({ page }) => {
    await page.getByText('Deployments').click();
    await page.getByText('default').click();
    await expect(page.getByText('web-app')).toBeVisible();
    await expect(page.getByText('api-server')).toBeVisible();
    await expect(page.getByText('standalone-pod')).not.toBeVisible();
  });

  test('expanding a namespace under Pods shows only Pod-kind workloads', async ({ page }) => {
    await page.getByText('Pods').click();
    await page.getByText('default').click();
    await expect(page.getByText('standalone-pod')).toBeVisible();
    await expect(page.getByText('web-app')).not.toBeVisible();
  });

  test('hides namespaces that have nothing of the chosen kind', async ({ page }) => {
    // kube-system only has a Deployment in the fixture, so it should not
    // appear at all under Pods (default has standalone-pod).
    await page.getByText('Pods').click();
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).not.toBeVisible();
  });

  test('shows "No <Kind>" when nothing in any namespace matches the chosen kind', async ({ page }) => {
    await page.getByText('StatefulSets').click();
    await expect(page.getByText('No StatefulSets')).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.getByText('kube-system')).not.toBeVisible();
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
    await expect(page.getByText('Deployments')).toBeVisible();
    await expect(page.getByText('Pods')).toBeVisible();
  });

  test('selecting a Deployment-kind workload shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Deployments').click();
    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // LogToolbar renders the workload name in a chip (sidebar uses ListItemText, not a chip)
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });

  test('selecting a Pod-kind workload shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('Pods').click();
    await page.getByText('default').click();
    await page.getByText('standalone-pod').click();

    await expect(page.locator('.MuiChip-root').filter({ hasText: 'standalone-pod' })).toBeVisible();
  });
});

test.describe('MobileSidebarNav', () => {
  test('only the Indexes and Workloads icons are visible, with nothing expanded yet', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Indexes' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Deployments' })).not.toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('tapping Workloads opens a kind picker, then drills into namespaces and a workload', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await page.getByRole('button', { name: 'Workloads' }).click();
    await expect(page.getByText('Deployments')).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();

    await page.getByText('Deployments').click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByText('default').click();
    await page.getByText('web-app').first().click();

    // The overlay auto-closes back down to just the bottom icons after a selection.
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
  });
});
