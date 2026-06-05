import { test, expect } from '@playwright/test';

test.describe('Indexes', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.getByRole('combobox').click();
    await page.getByRole('option', { name: 'Indexes' }).click();
  });

  test('lists values by count and drills into matching log messages', async ({ page }) => {
    await page.getByText('companyUuid').click();

    const company1 = page.getByRole('button', { name: /company-1/ });
    const company2 = page.getByRole('button', { name: /company-2/ });
    await expect(company1).toBeVisible();
    await expect(company1.getByText('2')).toBeVisible();
    await expect(company2).toBeVisible();
    await expect(company2.getByText('1')).toBeVisible();
    await expect(page.getByText(/log messages?/)).not.toBeVisible();
    await expect(page.getByPlaceholder('Filter results...')).not.toBeVisible();

    await company1.click();
    await expect(page.getByText('indexed web request')).toBeVisible();
    await expect(page.getByText('indexed api request')).toBeVisible();
    await expect(page.getByText('other company request')).not.toBeVisible();
    await expect(page.getByLabel('Value')).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Go' })).not.toBeVisible();
    await expect(page.getByPlaceholder('Filter results...')).toBeVisible();
  });

  test('manual autocomplete value search renders matching log messages', async ({ page }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    await page.getByText('companyUuid').click();
    await page.getByLabel('Value').fill('company-2');
    await page.getByRole('button', { name: 'Go' }).click();

    await expect(page.getByText('other company request')).toBeVisible();
    await expect(page.getByLabel('Value')).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Go' })).not.toBeVisible();
    await expect(page.getByPlaceholder('Filter results...')).toBeVisible();
    expect(pageErrors).toEqual([]);
  });
});
