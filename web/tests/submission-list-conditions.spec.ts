import { expect, test } from '@playwright/test'
import type { APIRequestContext } from '@playwright/test'

// The fixture API echoes the requests for this contest and this problem.
const contest = '66666666-6666-4666-8666-666666666666'
const echoProblem = 'd8d8d8d8-d8d8-4d8d-8d8d-d8d8d8d8d8d8'
const problem = '11111111-1111-4111-8111-111111111111'
const session = { Cookie: 'openoj_access=valid-access' }

const echoed = async (request: APIRequestContext, path: string, headers?: Record<string, string>) => {
  const response = await request.get(path, { headers })
  expect(response.ok()).toBe(true)
  const { route, query } = await response.json() as { route: string, query: string }
  return { route, params: [...new URLSearchParams(query)] }
}

test('the contest list route passes the conditions on', async ({ request }) => {
  const list = `/api/contests/${contest}/submissions`
  const all = await echoed(request, `${list}?sort=fastest&verdict=AC&language=cpp&user=alice&problem=${problem}&limit=20`)
  expect(all.params).toEqual([['offset', '0'], ['mine', '0'], ['limit', '20'], ['problem', problem], ['sort', 'fastest'], ['verdict', 'AC'], ['language', 'cpp'], ['user', 'alice']])
  // The runtime is passed in place of the language.
  expect((await echoed(request, `${list}?runtime=cpp23-gcc`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'cpp23-gcc']])
  expect((await echoed(request, `${list}?language=cpp&runtime=cpp23-gcc`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'cpp23-gcc']])
  // Values that cannot be valid are dropped.
  for (const bad of ['sort=longest', 'verdict=%3Cscript%3E', 'verdict=ac', 'user=a%20b', 'language=C%2B%2B', 'runtime=cpp23-gcc-isolate', 'runtime=cpp23-local', 'evil=1']) {
    expect((await echoed(request, `${list}?${bad}`)).params, bad).toEqual([['offset', '0'], ['mine', '0']])
  }
  // The sort and the verdict take every listed value.
  for (const sort of ['newest', 'oldest', 'shortest', 'fastest']) expect((await echoed(request, `${list}?sort=${sort}`)).params).toContainEqual(['sort', sort])
  for (const verdict of ['AC', 'WA', 'TLE', 'MLE', 'OLE', 'RE', 'CE', 'JE']) expect((await echoed(request, `${list}?verdict=${verdict}`)).params).toContainEqual(['verdict', verdict])
  // The signed-in routes and the contest problem's list receive the conditions too.
  const own = await echoed(request, `${list}?mine=1&scope=onsite&sort=oldest&verdict=WA`, session)
  expect(own.route).toBe('private')
  expect(own.params).toEqual([['offset', '0'], ['mine', '1'], ['scope', 'onsite'], ['sort', 'oldest'], ['verdict', 'WA']])
  const byProblem = await echoed(request, `/api/contests/${contest}/problems/${problem}/submissions?sort=shortest&language=python&user=bob&evil=1`)
  expect(byProblem.route).toBe('public')
  expect(byProblem.params).toEqual([['offset', '0'], ['mine', '0'], ['sort', 'shortest'], ['language', 'python'], ['user', 'bob']])
  const byProblemOwn = await echoed(request, `/api/contests/${contest}/problems/${problem}/submissions?mine=team&runtime=cpp17`, session)
  expect(byProblemOwn.route).toBe('private')
  expect(byProblemOwn.params).toEqual([['offset', '0'], ['mine', 'team'], ['runtime', 'cpp17']])
})

test('the problem list route passes the conditions on', async ({ request }) => {
  const list = `/api/problems/${echoProblem}/submissions`
  const all = await echoed(request, `${list}?offset=20&sort=fastest&verdict=TLE&language=cpp&user=alice&evil=1`)
  expect(all.route).toBe('public')
  expect(all.params).toEqual([['offset', '20'], ['mine', '0'], ['sort', 'fastest'], ['verdict', 'TLE'], ['language', 'cpp'], ['user', 'alice']])
  expect((await echoed(request, `${list}?runtime=python314&language=python`)).params).toEqual([['offset', '0'], ['mine', '0'], ['runtime', 'python314']])
  for (const bad of ['sort=longest', 'verdict=%3Cscript%3E', 'user=a%20b', 'language=C%2B%2B', 'runtime=cpp23-gcc-local']) {
    expect((await echoed(request, `${list}?${bad}`)).params, bad).toEqual([['offset', '0'], ['mine', '0']])
  }
  // With a session the private path receives the same conditions.
  const signedIn = await echoed(request, `${list}?sort=oldest&verdict=AC&runtime=cpp23-gcc&user=bob`, session)
  expect(signedIn.route).toBe('private')
  expect(signedIn.params).toEqual([['offset', '0'], ['mine', '0'], ['sort', 'oldest'], ['verdict', 'AC'], ['runtime', 'cpp23-gcc'], ['user', 'bob']])
  const mine = await echoed(request, `${list}?mine=1&sort=shortest`, session)
  expect(mine.route).toBe('private')
  expect(mine.params).toEqual([['offset', '0'], ['mine', '1'], ['sort', 'shortest']])
})
