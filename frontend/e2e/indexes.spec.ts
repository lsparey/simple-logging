import { test, expect } from '@playwright/test';

test.describe('Indexes', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.getByRole('combobox').click();
    await page.getByRole('option', { name: 'Indexes' }).click();
  });

  test('lists values by latest activity and drills into matching log messages', async ({ page }) => {
    await page.getByText('companyUuid').click();

    await expect(page.getByRole('button', { name: /index actions/i })).toBeVisible();
    const company1 = page.getByRole('button', { name: /company-1/ });
    const company2 = page.getByRole('button', { name: /company-2/ });
    await expect(company1).toBeVisible();
    await expect(company1.getByText('2')).toBeVisible();
    await expect(company2).toBeVisible();
    await expect(company2.getByText('1')).toBeVisible();
    await expect(page.getByText(/log messages?/)).not.toBeVisible();
    await expect(page.getByPlaceholder('Search…')).not.toBeVisible();

    await company1.click();
    await expect(page.getByText('indexed web request')).toBeVisible();
    await expect(page.getByText('indexed api request')).toBeVisible();
    await expect(page.getByText('other company request')).not.toBeVisible();
    await expect(page.getByLabel('Value', { exact: true })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Go' })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Back to index values' })).toBeVisible();
    await expect(page.locator('.MuiChip-root').filter({ hasText: 'company-1' })).toBeVisible();
    await expect(page.getByPlaceholder('Search…')).toBeVisible();
  });

  test('manual autocomplete value search renders matching log messages', async ({ page }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    await page.getByText('companyUuid').click();
    await page.getByLabel('Value', { exact: true }).fill('company-2');
    await page.getByRole('button', { name: 'Go' }).click();

    await expect(page.getByText('other company request')).toBeVisible();
    await expect(page.getByLabel('Value', { exact: true })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Go' })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Back to index values' })).toBeVisible();
    await expect(page.getByPlaceholder('Search…')).toBeVisible();
    expect(pageErrors).toEqual([]);
  });

  test('selected index menu can create another index and delete it', async ({ page }) => {
    const key = `tempDelete${Date.now()}`;

    await page.getByText('companyUuid').click();
    await page.getByRole('button', { name: /index actions/i }).click();
    await page.getByRole('menuitem', { name: 'Create another index' }).click();
    await page.getByLabel('JSON key').fill(key);
    await page.getByRole('button', { name: 'Create' }).click();

    await expect(page.locator('.MuiChip-root').filter({ hasText: key })).toBeVisible();
    await page.getByRole('button', { name: /index actions/i }).click();
    await page.getByRole('menuitem', { name: 'Delete index' }).click();

    await expect(page.getByText('Create or select an index to query JSON logs.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Index' })).toBeVisible();
    await expect(page.getByText(key)).not.toBeVisible();
  });
});
