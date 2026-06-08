import { test, expect } from '@playwright/test';

test.describe('Data dashboard', () => {
  test('shows log and index file sizes with modified dates', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Open data dashboard' }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole('heading', { name: 'Data dashboard' })).toBeVisible();
    await expect(page.getByText('5.00 KB')).toBeVisible();
    const table = page.getByRole('table', { name: 'Data file sizes' });
    await expect(table).toContainText('api-server-5b4c9e.log');
    await expect(table).toContainText('2.00 KB');
    await expect(table).toContainText('indexes.json');
    await expect(table).toContainText('keys/companyUuid/values/a1/company-1.jsonl');
    await expect(table).toContainText('Index');
    await expect(table.getByRole('columnheader', { name: 'Last updated' })).toBeVisible();
    await expect(table.getByRole('columnheader', { name: 'Bytes' })).toHaveCount(0);
  });
});
