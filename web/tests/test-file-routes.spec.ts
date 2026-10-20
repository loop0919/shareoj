import { request as httpRequest } from 'node:http'
import { expect, test } from '@playwright/test'

// The fixture API echoes the requests for this problem, for not-a-problem, and for the paths a dot segment in an ID leads to.
const problem = '12121212-1212-4212-8212-121212121212'
const file = '11111111-1111-4111-8111-111111111111'
const session = { cookie: 'openoj_access=valid-access', origin: 'https://judge.example' }
const begin = { size: 1, sha256: 'a'.repeat(64) }

// Playwright's request resolves dot segments such as %2e%2e before sending, so this sends the path as written.
function send(method: 'GET' | 'POST', path: string, headers: Record<string, string>, body?: unknown) {
  return new Promise<{ status: number, body: string }>((resolve, reject) => {
    const request = httpRequest({ host: '127.0.0.1', port: 13000, method, path, headers: body === undefined ? headers : { ...headers, 'content-type': 'application/json' } }, response => {
      let text = ''
      response.setEncoding('utf8')
      response.on('data', chunk => { text += chunk })
      response.on('end', () => resolve({ status: response.statusCode ?? 0, body: text }))
    })
    request.on('error', reject)
    request.end(body === undefined ? undefined : JSON.stringify(body))
  })
}

test('test file routes check their IDs before building the API path', async () => {
  const echo = async (method: 'GET' | 'POST', path: string, body?: unknown) => {
    const response = await send(method, path, session, body)
    expect(response.status, `${method} ${path}`).toBe(200)
    return JSON.parse(response.body) as { path: string, url: string, method: string, body: string }
  }
  const getPath = `/my/problems/${problem}/test-files/${file}`
  expect(await echo('GET', `/api${getPath}`)).toMatchObject({ path: getPath, url: getPath, method: 'GET' })
  expect(await echo('POST', `/api${getPath}/complete`)).toMatchObject({ path: `${getPath}/complete`, url: `${getPath}/complete`, method: 'POST' })
  const begun = await echo('POST', `/api/my/problems/${problem}/test-files`, begin)
  expect(begun).toMatchObject({ path: `/my/problems/${problem}/test-files`, url: `/my/problems/${problem}/test-files`, method: 'POST' })
  expect(JSON.parse(begun.body)).toEqual(begin)
  expect((await send('POST', `/api/my/problems/${problem}/test-files`, session, { ...begin, sha256: 'a'.repeat(3000) })).status).toBe(413)

  // Each answers 404 without reaching the API: had it reached the fixture, the echo would answer 200 with the path it received.
  const rejected: ['GET' | 'POST', string, unknown?][] = [
    ['GET', `/api/my/problems/${problem}/test-files/%2e%2e`],
    ['GET', `/api/my/problems/${problem}/test-files/.%2E`],
    ['GET', `/api/my/problems/${problem}/test-files/not-a-test-file`],
    ['GET', `/api/my/problems/%2e%2e/test-files/${file}`],
    ['GET', `/api/my/problems/not-a-problem/test-files/${file}`],
    ['POST', `/api/my/problems/${problem}/test-files/%2E%2e/complete`],
    ['POST', `/api/my/problems/${problem}/test-files/not-a-test-file/complete`],
    ['POST', `/api/my/problems/%2e%2e/test-files/${file}/complete`],
    ['POST', `/api/my/problems/not-a-problem/test-files/${file}/complete`],
    ['POST', '/api/my/problems/%2e%2e/test-files', begin],
    ['POST', '/api/my/problems/not-a-problem/test-files', begin],
  ]
  const answers: Record<string, number | string> = {}
  for (const [method, path, body] of rejected) {
    const response = await send(method, path, session, body)
    answers[`${method} ${path}`] = response.status === 404 ? 404 : `${response.status} ${response.body}`
  }
  expect(answers).toEqual(Object.fromEntries(rejected.map(([method, path]) => [`${method} ${path}`, 404])))

  // The session and origin checks still come first.
  const attacker = { ...session, origin: 'https://attacker.example' }
  expect((await send('GET', `/api/my/problems/%2e%2e/test-files/${file}`, {})).status).toBe(401)
  expect((await send('POST', `/api/my/problems/%2e%2e/test-files/${file}/complete`, {})).status).toBe(401)
  expect((await send('POST', '/api/my/problems/%2e%2e/test-files', {}, begin)).status).toBe(401)
  expect((await send('POST', `/api/my/problems/%2e%2e/test-files/${file}/complete`, attacker)).status).toBe(403)
  expect((await send('POST', '/api/my/problems/%2e%2e/test-files', attacker, begin)).status).toBe(403)
})
