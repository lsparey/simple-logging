import { test, expect } from '@playwright/test';

test.describe('Indexes', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.getByRole('combobox').click();
    await page.getByRole('option', { name: 'Indexes' }).click();
  });

  test('lists values by count and drills into matching log messages', async ({ page }) => {
    await page.getByText('companyUuid').click();

    await expect(page.getByText('company-1')).toBeVisible();
    await expect(page.getByText('2 log messages')).toBeVisible();
    await expect(page.getByText('company-2')).toBeVisible();
    await expect(page.getByText('1 log message')).toBeVisible();

    await page.getByText('company-1').click();
    await expect(page.getByText('indexed web request')).toBeVisible();
    await expect(page.getByText('indexed api request')).toBeVisible();
    await expect(page.getByText('other company request')).not.toBeVisible();
  });

  test('manual value search renders matching log messages', async ({ page }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    await page.getByText('companyUuid').click();
    await page.getByLabel('Value').fill('company-2');
    await page.getByRole('button', { name: 'Search' }).click();

    await expect(page.getByText('other company request')).toBeVisible();
    expect(pageErrors).toEqual([]);
  });
});
