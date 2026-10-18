import { expect, test } from '@playwright/test'

// The fixture API echoes the requests for this contest.
const id = '66666666-6666-4666-8666-666666666666'
const problem = '11111111-1111-4111-8111-111111111111'
const session = { Cookie: 'openoj_access=valid-access' }

test('the web server forwards only the onsite filter and asks for the team list with a session (ADR 0024)', async ({ request }) => {
  const forwarded = async (path: string, headers?: Record<string, string>) => {
    const response = await request.get(`/api/contests/${id}/${path}`, { headers })
    if (!response.ok()) return response.status()
    const { route, query } = await response.json() as { route: string, query: string }
    return `${route} ${query}`
  }
  expect(await forwarded('standings?scope=onsite')).toBe('public ?offset=0&mine=0&scope=onsite')
  expect(await forwarded('standings?scope=everyone')).toBe('public ?offset=0&mine=0')
  expect(await forwarded('standings?scope=onsite', session)).toBe('public ?offset=0&mine=0&scope=onsite')
  expect(await forwarded('standings?live=1&scope=onsite', session)).toBe('private ?live=1&scope=onsite')
  expect(await forwarded('standings?live=1&scope=all', session)).toBe('private ?live=1')
  expect(await forwarded('submissions?offset=50&scope=onsite')).toBe('public ?offset=50&mine=0&scope=onsite')
  expect(await forwarded('submissions?scope=onsite', session)).toBe('private ?offset=0&mine=0&scope=onsite')
  expect(await forwarded(`problems/${problem}/submissions?scope=onsite`)).toBe('public ?offset=0&mine=0&scope=onsite')
  expect(await forwarded(`problems/${problem}/submissions?mine=team`)).toBe(401)
  expect(await forwarded(`problems/${problem}/submissions?mine=team&scope=onsite`, session)).toBe('private ?offset=0&mine=team&scope=onsite')
})
