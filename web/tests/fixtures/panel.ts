import type { Page } from '@playwright/test'

// The page clips what sticks out sideways, so a check of the document's width cannot see a control wider than the
// conditions panel. This lists the panel's controls that reach outside it.
export const outsidePanel = (page: Page) => page.evaluate(() => {
  const panel = document.querySelector('.conditions-panel')!
  const box = panel.getBoundingClientRect()
  return [...panel.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>('select, input, button')]
    .filter(control => { const own = control.getBoundingClientRect(); return own.left < box.left - 0.5 || own.right > box.right + 0.5 })
    .map(control => control.labels?.[0]?.textContent ?? control.getAttribute('aria-label') ?? control.textContent ?? control.tagName)
})
