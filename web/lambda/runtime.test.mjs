import { afterEach, expect, test } from 'bun:test'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:http'
import { mkdtemp, mkdir, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const handlerSource = `
export async function handler(event, context) {
  if (event.fail) throw new TypeError('fixture failure')
  if (event.log) console.log('fixture log\\nsecond line', { id: context.awsRequestId })
  if (event.large) return { body: 'x'.repeat(64) }
  return { id: context.awsRequestId, function: context.functionName, remaining: context.getRemainingTimeInMillis() > 0, trace: process.env._X_AMZN_TRACE_ID ?? null, bun: Boolean(process.versions.bun) }
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
    { id: 'first', body: {}, headers: { 'lambda-runtime-trace-id': 'Root=1-test' } },
    { id: 'second', body: {} },
  ])
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 2)
  expect(api.posts[0]).toEqual({ path: 'invocation/first/response', errorType: undefined, body: { id: 'first', function: 'judge-test-web', remaining: true, trace: 'Root=1-test', bun: true } })
  expect(api.posts[1].body.trace).toBeNull()
})

test('reports a thrown error for that invocation and keeps serving', async () => {
  const api = await runtimeAPI([{ id: 'broken', body: { fail: true } }, { id: 'after', body: {} }])
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 2)
  expect(api.posts[0].path).toBe('invocation/broken/error')
  expect(api.posts[0].errorType).toBe('Unhandled')
  expect(api.posts[0].body).toMatchObject({ errorType: 'TypeError', errorMessage: 'fixture failure' })
  expect(api.posts[1].path).toBe('invocation/after/response')
})

test('writes log lines as the Node.js runtime does, one event per call', async () => {
  const api = await runtimeAPI([{ id: 'logged', body: { log: true } }])
  const { output } = start(await taskRoot(), api.address)
  await until(() => api.posts.length === 1 && output.stdout.includes('fixture log'))
  expect(output.stdout).toMatch(/^\d{4}-\d\d-\d\dT[\d:.]+Z\tlogged\tINFO\tfixture log\rsecond line \{ id: 'logged' \}\n$/m)
})

test('reports a handler that cannot load as an initialization error', async () => {
  const api = await runtimeAPI([])
  const { child } = start(await taskRoot("throw new Error('cannot load')"), api.address)
  const [code] = await once(child, 'exit')
  expect(code).toBe(1)
  expect(api.posts).toEqual([{ path: 'init/error', errorType: 'Runtime.ImportModuleError', body: expect.objectContaining({ errorType: 'Error', errorMessage: 'cannot load' }) }])
})

test('asks for the next invocation again after the Runtime API drops the connection', async () => {
  const api = await runtimeAPI([{ id: 'retried', body: {} }], { dropFirstNext: true })
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 1)
  expect(api.posts[0].path).toBe('invocation/retried/response')
})

test('reports an invocation as failed when Lambda refuses its response', async () => {
  const api = await runtimeAPI([{ id: 'too-large', body: { large: true } }, { id: 'next', body: {} }], { rejectResponseOf: 'too-large' })
  start(await taskRoot(), api.address)
  await until(() => api.posts.length === 3)
  expect(api.posts.map(post => post.path)).toEqual(['invocation/too-large/response', 'invocation/too-large/error', 'invocation/next/response'])
})
