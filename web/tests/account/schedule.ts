import { expect, type APIRequestContext } from '@playwright/test'

const fixtureAPI = `http://${process.env.OPENOJ_BROWSER_API_ADDR ?? '127.0.0.1:18082'}`

// Moves a contest's start and end to seconds from now through the browser fixture API instead of
// waiting for them. The API decides a contest's state from these times, so the contest goes
// through the same code as one that waited. Keep the end after the start, and end at 0 rather than
// in the past, or the latest runs would fall after the end.
export async function moveContest(request: APIRequestContext, id: string, schedule: { startsIn?: number, endsIn?: number }) {
  const response = await request.post(`${fixtureAPI}/__test/contests/${id}/schedule`, { data: schedule })
  expect(response.status(), await response.text()).toBe(204)
}
