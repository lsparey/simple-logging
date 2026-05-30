import type { Page } from '@playwright/test';

/** Navigate to the app, switch to deployments view, and click a deployment. */
export async function selectDeployment(page: Page, namespace: string, deployment: string) {
  await page.goto('/');
  // Ensure deployments view mode is active (it's the default, but be explicit)
  const combo = page.getByRole('combobox');
  const current = await combo.inputValue().catch(() => null) ?? await combo.textContent();
  if (!current?.toLowerCase().includes('deployment')) {
    await combo.click();
    await page.getByRole('option', { name: 'Deployments' }).click();
  }
  await page.getByText(namespace).click();
  await page.getByText(deployment).first().click();
}

/** Navigate to the app, switch to pods view, and click a pod. */
export async function selectPod(page: Page, namespace: string, pod: string) {
  await page.goto('/');
  await page.getByRole('combobox').click();
  await page.getByRole('option', { name: 'Pods' }).click();
  await page.getByText(namespace).click();
  await page.getByText(pod).click();
}

/**
 * Scroll the react-window log list to the very top by walking up from the
 * first rendered <pre> element to find its overflow:auto container and
 * setting scrollTop = 0 programmatically.
 */
export async function scrollLogListToTop(page: Page) {
  await page.evaluate(() => {
    const pre = document.querySelector('pre');
    if (!pre) return;
    let el: Element | null = pre.parentElement;
    while (el instanceof HTMLElement) {
      if (el.style.overflow === 'auto' || el.style.overflowY === 'auto') {
        el.scrollTop = 0;
        return;
      }
      el = el.parentElement;
    }
  });
}
