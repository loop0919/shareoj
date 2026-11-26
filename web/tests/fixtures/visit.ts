import type { Page } from '@playwright/test'

// Moves to a path with the app's router, as a link from elsewhere in the app would, without loading the page again.
export async function visit(page: Page, path: string) {
  await page.evaluate(to => (window as unknown as { useNuxtApp: () => { $router: { push: (to: string) => Promise<unknown> } } }).useNuxtApp().$router.push(to), path)
}
