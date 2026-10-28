import { afterEach, expect, test } from 'bun:test'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:http'
import { mkdtemp, mkdir, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

// The expectations follow the Node.js runtime's own client (/var/runtime in the nodejs22.x image):
// its log lines, its error reports, and its exits when an error escapes the handler.
const handlerSource = `
export async function handler(event, context) {
  if (event.fail) throw new TypeError('fixture failure')
  if (event.reject) { Promise.reject(new TypeError('stray')); return new Promise(() => {}) }
  if (event.throwLater) { setTimeout(() => { throw new RangeError('later') }, 0); return new Promise(() => {}) }
  if (event.log) {
    console.log('fixture log\\nsecond line', { id: context.awsRequestId })
    console.trace('fixture trace')
    console.fatal('fixture fatal')
    setTimeout(() => console.info('after the response'), 50)
  }
  if (event.large) return { body: 'x'.repeat(64) }
  return {
    id: context.awsRequestId, function: context.functionName, remaining: context.getRemainingTimeInMillis() > 0,
    trace: process.env._X_AMZN_TRACE_ID ?? null, bun: Boolean(process.versions.bun),
    clientContext: context.clientContext ?? null, identity: context.identity ?? null, tenant: context.tenantId ?? null,
  }
}`

const cleanups = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A task root laid out like the ZIP, with the real bootstrap and runtime and this test's Bun.
async function taskRoot(handler = handlerSource) {
  const root = await mkdtemp(join(tmpdir(), 'lambda-runtime-'))
  cleanups.push(() => rm(root, { recursive: true, force: true }))
  await mkdir(join(root, 'bin'))
  await mkdir(join(root, 'lambda'))
  await symlink(process.execPath, join(root, 'bin/bun'))
  await symlink(fileURLToPath(new URL('./bootstrap', import.meta.url)), join(root, 'bootstrap'))
  await symlink(fileURLToPath(new URL('./runtime.mjs', import.meta.url)), join(root, 'lambda/runtime.mjs'))
  await writeFile(join(root, 'lambda/index.mjs'), handler)
  return root
}

// A Runtime API that hands out `events` in order, records what the runtime posts, and then holds.
async function runtimeAPI(events, { dropFirstNext = false, rejectResponseOf } = {}) {
  const posts = []
  let next = 0
  let dropped = false
  const server = createServer(async (req, res) => {
    const path = req.url.replace('/2018-06-01/runtime/', '')
    if (path === 'invocation/next') {
      if (dropFirstNext && !dropped) { dropped = true; req.socket.destroy(); return }
      const event = events[next++]
      if (!event) return // Hold the long poll, as Lambda does until the next request.
      res.writeHead(200, { 'lambda-runtime-aws-request-id': event.id, 'lambda-runtime-deadline-ms': String(Date.now() + 10_000), ...event.headers })
      res.end(JSON.stringify(event.body))
      return
    }
    const chunks = []
    for await (const chunk of req) chunks.push(chunk)
    posts.push({ path, errorType: req.headers['lambda-runtime-function-error-type'], body: JSON.parse(Buffer.concat(chunks).toString() || 'null') })
    res.writeHead(path === `invocation/${rejectResponseOf}/response` ? 413 : 202).end()
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  cleanups.push(() => { server.closeAllConnections(); return new Promise(resolve => server.close(resolve)) })
  return { posts, address: `127.0.0.1:${server.address().port}` }
}

function start(root, address) {
  const child = spawn(join(root, 'bootstrap'), [], {
    env: { PATH: process.env.PATH, AWS_LAMBDA_RUNTIME_API: address, LAMBDA_TASK_ROOT: root, _HANDLER: 'lambda/index.handler', AWS_LAMBDA_FUNCTION_NAME: 'judge-test-web' },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const output = { stdout: '', stderr: '' }
  child.stdout.on('data', chunk => { output.stdout += chunk })
  child.stderr.on('data', chunk => { output.stderr += chunk })
  cleanups.push(() => { if (child.exitCode === null) child.kill('SIGKILL') })
  return { child, output }
}

async function until(condition) {
  for (let i = 0; i < 200 && !condition(); i++) await Bun.sleep(25)
  expect(condition()).toBe(true)
}

test('answers invocations with the handler result, its context, and the trace header', async () => {
  const api = await runtimeAPI([
    {
      id: 'first', body: {},
      headers: {
        'lambda-runtime-trace-id': 'Root=1-test', 'lambda-runtime-client-context': '{"custom":{"app":"web"}}',
        'lambda-runtime-cognito-identity': '{"cognitoIdentityId":"id-1"}', 'lambda-runtime-aws-tenant-id': 'tenant-1',
      },
    },
    { id: 'second', body: {} },
  ])
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 2)
  expect(api.posts[0]).toEqual({
    path: 'invocation/first/response', errorType: undefined,
    body: { id: 'first', function: 'judge-test-web', remaining: true, trace: 'Root=1-test', bun: true, clientContext: { custom: { app: 'web' } }, identity: { cognitoIdentityId: 'id-1' }, tenant: 'tenant-1' },
  })
  expect(api.posts[1].body).toMatchObject({ trace: null, clientContext: null, identity: null, tenant: null })
})

test('reports a thrown error for that invocation as the Node.js runtime does, and keeps serving', async () => {
  const api = await runtimeAPI([{ id: 'broken', body: { fail: true } }, { id: 'after', body: {} }])
  const { output } = start(await taskRoot(), api.address)
  await until(() => api.posts.length === 2)
  expect(api.posts[0]).toMatchObject({ path: 'invocation/broken/error', errorType: 'TypeError', body: { errorType: 'TypeError', errorMessage: 'fixture failure' } })
  expect(api.posts[0].body.trace[0]).toBe('TypeError: fixture failure')
  expect(output.stdout).toMatch(/\tbroken\tERROR\tInvoke Error \t\{"errorType":"TypeError","errorMessage":"fixture failure","stack":\["TypeError: fixture failure",/)
  expect(api.posts[1].path).toBe('invocation/after/response')
})

test('writes log lines as the Node.js runtime does, one event per call', async () => {
  const api = await runtimeAPI([{ id: 'logged', body: { log: true } }])
  const { output } = start(await taskRoot(), api.address)
  await until(() => api.posts.length === 1 && output.stdout.includes('after the response'))
  expect(output.stdout).toMatch(/^\d{4}-\d\d-\d\dT[\d:.]+Z\tlogged\tINFO\tfixture log\rsecond line \{ id: 'logged' \}\n/m)
  expect(output.stdout).toMatch(/\tlogged\tTRACE\tfixture trace\n/)
  expect(output.stdout).toMatch(/\tlogged\tFATAL\tfixture fatal\n/)
  // The last request ID stays on lines written after the response.
  expect(output.stdout).toMatch(/\tlogged\tINFO\tafter the response\n/)
})

test('reports a handler that throws while loading as an initialization error, and exits with 129', async () => {
  const api = await runtimeAPI([])
  const { child, output } = start(await taskRoot("throw new Error('cannot load')"), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(129)
  expect(api.posts).toEqual([{ path: 'init/error', errorType: 'Error', body: expect.objectContaining({ errorType: 'Error', errorMessage: 'cannot load' }) }])
  expect(output.stdout).toMatch(/\tundefined\tERROR\tUncaught Exception \t\{"errorType":"Error","errorMessage":"cannot load"/)
})

test('names a handler that the module does not export as the Node.js runtime does', async () => {
  const api = await runtimeAPI([])
  const { child } = start(await taskRoot('export const other = () => {}'), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(129)
  expect(api.posts).toEqual([{ path: 'init/error', errorType: 'Runtime.HandlerNotFound', body: expect.objectContaining({ errorType: 'Runtime.HandlerNotFound', errorMessage: 'lambda/index.handler is undefined or not exported' }) }])
})

test('reports an unhandled rejection as the invocation error, and exits with 128', async () => {
  const api = await runtimeAPI([{ id: 'stray', body: { reject: true } }])
  const { child, output } = start(await taskRoot(), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(128)
  expect(api.posts).toEqual([{ path: 'invocation/stray/error', errorType: 'Runtime.UnhandledPromiseRejection', body: expect.objectContaining({ errorType: 'Runtime.UnhandledPromiseRejection', errorMessage: 'TypeError: stray' }) }])
  expect(output.stdout).toMatch(/\tstray\tERROR\tUnhandled Promise Rejection \t\{"errorType":"Runtime.UnhandledPromiseRejection","errorMessage":"TypeError: stray"/)
})

test('reports an uncaught exception as the invocation error, and exits with 129', async () => {
  const api = await runtimeAPI([{ id: 'thrown', body: { throwLater: true } }])
  const { child, output } = start(await taskRoot(), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(129)
  expect(api.posts).toEqual([{ path: 'invocation/thrown/error', errorType: 'RangeError', body: expect.objectContaining({ errorType: 'RangeError', errorMessage: 'later' }) }])
  expect(output.stdout).toMatch(/\tthrown\tERROR\tUncaught Exception \t\{"errorType":"RangeError","errorMessage":"later"/)
})

test('stops when the next invocation cannot be read, so that Lambda starts a new runtime', async () => {
  const api = await runtimeAPI([{ id: 'never', body: {} }], { dropFirstNext: true })
  const { child, output } = start(await taskRoot(), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(129)
  expect(api.posts.map(post => post.path)).toEqual(['init/error'])
  expect(output.stdout).toContain('Unexpected Top Level Error: ')
  expect(output.stdout).not.toContain('Uncaught Exception')
})

test('escapes DEL in a reported error, as the Node.js runtime does, so that its type fits a header', async () => {
  const api = await runtimeAPI([{ id: 'odd', body: { odd: true } }])
  start(await taskRoot(`export async function handler() {
  const error = new Error('one\\x7Ftwo')
  error.name = 'Odd\\x7FError'
  throw error
}`), api.address)
  await until(() => api.posts.length === 1)
  expect(api.posts[0]).toMatchObject({ path: 'invocation/odd/error', errorType: 'Odd%7FError', body: { errorType: 'Odd%7FError', errorMessage: 'one%7Ftwo' } })
})

test('reports an invocation as failed when Lambda refuses its response', async () => {
  const api = await runtimeAPI([{ id: 'too-large', body: { large: true } }, { id: 'next', body: {} }], { rejectResponseOf: 'too-large' })
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 3)
  expect(api.posts.map(post => post.path)).toEqual(['invocation/too-large/response', 'invocation/too-large/error', 'invocation/next/response'])
})

test('keeps answering when the handler replaces the global fetch', async () => {
  const api = await runtimeAPI([{ id: 'first', body: {} }, { id: 'second', body: {} }])
  start(await taskRoot(`globalThis.fetch = () => Promise.reject(new Error('replaced fetch'))\n${handlerSource}`), api.address)
  await until(() => api.posts.length === 2)
  expect(api.posts.map(post => post.path)).toEqual(['invocation/first/response', 'invocation/second/response'])
})
