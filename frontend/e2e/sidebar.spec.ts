import { test, expect } from '@playwright/test';

test.describe('PodSidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('shows Indexes and every namespace at the top level, with nothing expanded yet', async ({ page }) => {
    await expect(page.getByText('Indexes')).toBeVisible();
    await expect(page.getByText('default')).toBeVisible();
    await expect(page.getByText('kube-system')).toBeVisible();
    await expect(page.getByText('Deployments')).not.toBeVisible();
  });

  test('expanding a namespace shows only the workload kinds present in it', async ({ page }) => {
    await page.getByText('default').click();
    await expect(page.getByText('Deployments')).toBeVisible();
    await expect(page.getByText('StatefulSets')).toBeVisible();
    await expect(page.getByText('Pods')).toBeVisible();
    await expect(page.getByText('DaemonSets')).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Jobs', exact: true })).not.toBeVisible();
    await expect(page.getByText('CronJobs')).not.toBeVisible();
  });

  test('expanding a kind shows its workloads', async ({ page }) => {
    await page.getByText('default').click();
    await page.getByText('Deployments').click();
    await expect(page.getByText('web-app')).toBeVisible();
    await expect(page.getByText('api-server')).toBeVisible();
  });

  test('the Pods kind lists every pod, including ones owned by a Deployment', async ({ page }) => {
    await page.getByText('default').click();
    await page.getByText('Pods').click();
    // standalone-pod has no owner; web-app-6d8c7f is owned by the web-app
    // Deployment. Both are individual pods, so both show up here.
    await expect(page.getByText('standalone-pod')).toBeVisible();
    await expect(page.getByText('web-app-6d8c7f')).toBeVisible();
  });

  test('collapsing a namespace hides its kind rows', async ({ page }) => {
    await page.getByText('default').click();
    await expect(page.getByText('Deployments')).toBeVisible();

    await page.getByText('default').click();
    await expect(page.getByText('Deployments')).not.toBeVisible();
  });

  test('selecting a Deployment-kind workload shows the log panel toolbar header', async ({ page }) => {
    await page.getByText('default').click();
    await page.getByText('Deployments').click();
    await page.getByText('web-app').first().click();

    // LogToolbar renders the workload name in a chip (sidebar uses ListItemText, not a chip)
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
  });

  test('selecting a Deployment-owned pod under Pods scopes the log view to that pod alone', async ({ page }) => {
    await page.getByText('default').click();
    await page.getByText('Pods').click();
    await page.getByText('web-app-6d8c7f').click();

    // The toolbar shows the pod's own name, not the owning Deployment's.
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app-6d8c7f' })).toBeVisible();
  });

  test('clicking Indexes shows a back arrow and hides the namespace tree; back restores it', async ({ page }) => {
    await page.getByText('Indexes').click();
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Back' })).toBeVisible();

    await page.getByRole('button', { name: 'Back' }).click();
    await expect(page.getByText('default')).toBeVisible();
  });
});

test.describe('MobileSidebarNav', () => {
  test('only the Indexes and Workloads icons are visible, with nothing expanded yet', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Indexes' })).toBeVisible();
    await expect(page.getByText('default')).not.toBeVisible();
  });

  test('tapping Workloads opens the namespace tree directly, then drills into a kind and a workload', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await page.getByRole('button', { name: 'Workloads' }).click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByText('default').click();
    await expect(page.getByText('Deployments')).toBeVisible();

    await page.getByText('Deployments').click();
    await page.getByText('web-app').first().click();

    // The overlay auto-closes back down to just the bottom icons after a selection.
    await expect(page.getByText('default')).not.toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'web-app' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Workloads' })).toBeVisible();
  });

  test('the back arrow from the Workloads drawer closes it back to the icons', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    await page.getByRole('button', { name: 'Workloads' }).click();
    await expect(page.getByText('default')).toBeVisible();

    await page.getByRole('button', { name: 'Back' }).click();
    await expect(page.getByText('default')).not.toBeVisible();
  });
});
