import type { Page } from '@playwright/test';
import { WORKLOAD_KIND_SECTIONS, type WorkloadKind } from '../src/components/PodSidebar/sidebarSections.js';

function kindSectionLabel(kind: WorkloadKind): string {
  const section = WORKLOAD_KIND_SECTIONS.find((s) => s.key === kind);
  if (!section) throw new Error(`unknown workload kind: ${kind}`);
  return section.label;
}

/** Navigate to the app, expand a namespace, expand a workload kind within it, and click a workload. */
export async function selectWorkload(page: Page, namespace: string, kind: WorkloadKind, name: string) {
  await page.goto('/');
  await page.getByText(namespace).click();
  await page.getByText(kindSectionLabel(kind)).click();
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
