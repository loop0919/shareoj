import { test as base } from '@playwright/test'

// The account suite's Cognito domain does not exist, so answer its /logout the way Cognito does:
// clear nothing and send the browser on to logout_uri.
export const test = base.extend({
  context: async ({ context }, use) => {
    await context.route('https://*.amazoncognito.com/logout?*', route => route.fulfill({
      status: 302, headers: { location: new URL(route.request().url()).searchParams.get('logout_uri')! },
    }))
    await use(context)
  },
})
export { expect } from '@playwright/test'
