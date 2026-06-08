import { test, expect } from '@playwright/test';

test.describe('Data dashboard', () => {
  test('opens from the app bar and shows file sizes and the total', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Open data dashboard' }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole('heading', { name: 'Data dashboard' })).toBeVisible();
    await expect(page.getByText('4.00 KB')).toBeVisible();
    await expect(page.getByRole('table', { name: 'Log file sizes' })).toContainText('api-server-5b4c9e.log');
    await expect(page.getByRole('table', { name: 'Log file sizes' })).toContainText('2.00 KB');
    await expect(page.getByRole('table', { name: 'Log file sizes' })).toContainText('512 B');
  });
});
