import type { Page } from '@playwright/test';

/** Navigate to the app, open the workloads view, expand a namespace, and click a workload. */
export async function selectWorkload(page: Page, namespace: string, name: string) {
  await page.goto('/');
  await page.getByText('Workloads').click();
  await page.getByText(namespace).click();
  await page.getByText(name).first().click();
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
