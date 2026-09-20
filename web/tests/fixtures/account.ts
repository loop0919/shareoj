import { test as base, expect } from '@playwright/test'

// UI tests use a per-test API fixture; account/ tests exercise the real Go API and DB.
export const test = base.extend({
  page: async ({ page }, use) => {
    const problems = new Map<string, { id: string, version: number, updatedAt: string, draft: Record<string, string> }>()
    await page.route('**/api/my/difficulty-votes/*', route => route.fulfill({ json: { difficulty: null, difficultyAverage: null, difficultyVoteCount: 0, difficultyDistribution: Array(10).fill(0) } }))
    await page.route('**/api/my/solved-problems', route => route.fulfill({ json: { items: [] } }))
    await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
    await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'ui_test', avatar: '', version: 1, createdAt: '2026-09-10T00:00:00Z' } } }))
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'ui-test-user' } } }))
    await page.route('**/api/my/problems**', async route => {
      const request = route.request()
      const id = new URL(request.url()).pathname.split('/')[4]
      if (!id) return route.fulfill({ json: { items: [...problems.values()].reverse().map(p => ({ id: p.id, title: p.draft.title, updatedAt: p.updatedAt })), nextCursor: '' } })
      if (request.method() === 'PUT') {
        const body = request.postDataJSON()
        const entry = { id, version: (problems.get(id)?.version ?? 0) + 1, updatedAt: new Date().toISOString(), draft: body.draft }
        problems.delete(id); problems.set(id, entry)
        return route.fulfill({ json: entry })
      }
      if (request.method() === 'DELETE') { problems.delete(id); return route.fulfill({ status: 204 }) }
      return route.fulfill(problems.has(id) ? { json: problems.get(id) } : { status: 404, json: {} })
    })
    await use(page)
  },
})
export { expect }
