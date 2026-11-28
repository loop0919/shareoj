import type { Locator, Page } from '@playwright/test'

// Picks an option in each select, or clicks each button, one right after another inside the page, so that each action
// comes while the move of the one before is still on its way, however fast the machine is. In one task, the route has not
// started to move; in the next task, the move is running Nuxt's route middleware.
export type QuickAction = { target: Locator, pick?: string }
export async function quickly(page: Page, actions: QuickAction[], { nextTask = false } = {}) {
  const steps = await Promise.all(actions.map(async ({ target, pick }) => ({ element: await target.elementHandle(), pick })))
  await page.evaluate(async ({ steps, nextTask }) => {
    for (const [index, { element, pick }] of steps.entries()) {
      if (!(element instanceof HTMLElement)) throw new Error('not an element')
      if (nextTask && index) await new Promise(resolve => setTimeout(resolve))
      if (pick === undefined) {
        element.click()
      }
      else {
        (element as HTMLSelectElement).value = pick
        element.dispatchEvent(new Event('change', { bubbles: true }))
      }
    }
  }, { steps, nextTask })
}
export const quickModes = [['in one task', false], ['in the next task', true]] as const
