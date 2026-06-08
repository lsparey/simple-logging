import { test, expect } from '@playwright/test';

test.describe('Storage dashboard', () => {
  test('shows log and index file sizes with modified dates', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Open storage dashboard' }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole('heading', { name: 'Storage dashboard' })).toBeVisible();
    await expect(page.getByText('5.00 KB')).toBeVisible();
    await expect(page.getByText('Log files', { exact: true }).locator('..')).toContainText('3');
    await expect(page.getByText('Index files', { exact: true }).locator('..')).toContainText('2');
    const table = page.getByRole('table', { name: 'Data file sizes' });
    await expect(table).toContainText('api-server-5b4c9e.log');
    await expect(table).toContainText('2.00 KB');
    await expect(table).toContainText('indexes.json');
    await expect(table).toContainText('keys/Y29tcGFueVV1aWQ/values/a1/company-1.jsonl');
    await expect(table).toContainText('Index');
    await expect(table).toContainText('default / api-server-5b4c9e');
    await expect(table).toContainText('companyUuid = company-1');
    await expect(table.getByRole('columnheader', { name: 'Subject' })).toBeVisible();
    await expect(table.getByRole('columnheader', { name: 'Namespace' })).toHaveCount(0);
    await expect(table.getByRole('columnheader', { name: 'Last updated' })).toBeVisible();
    await expect(table.getByRole('columnheader', { name: 'Bytes' })).toHaveCount(0);
  });
});
