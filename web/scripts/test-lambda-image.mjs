// Boot the packaged zip as Lambda does: AWS's base image and Runtime Interface Emulator, the
// zip's file modes, a read-only /var/task and root filesystem, a non-root user, and no HOME.
// usage: bun scripts/test-lambda-image.mjs [zip] [image]
// The default image is the provided.al2023 runtime; public.ecr.aws/lambda/nodejs:22 checks
// that the same zip still runs on the Node.js runtime.
import assert from 'node:assert/strict'
import { execFileSync, spawn, spawnSync } from 'node:child_process'
import { chmod, mkdtemp, readdir, rm } from 'node:fs/promises'
import { arch, tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const zip = process.argv[2] ?? fileURLToPath(new URL('../.build/web.zip', import.meta.url))
const image = process.argv[3] ?? 'public.ecr.aws/lambda/provided:al2023'
const port = 19080
const container = `openoj-lambda-image-${process.pid}`
const task = await mkdtemp(join(tmpdir(), 'openoj-lambda-image-'))
const containerLogs = () => {
  const logs = spawnSync('docker', ['logs', container], { encoding: 'utf8' })
  return `${logs.stdout ?? ''}${logs.stderr ?? ''}`
}
let api
try {
  // Apply the modes the zip records, as Lambda does.
  execFileSync('python3', ['-c', `import os, sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as bundle:
    for info in bundle.infolist():
        path = bundle.extract(info, sys.argv[2])
        if info.external_attr >> 16:
            os.chmod(path, info.external_attr >> 16 & 0o777)`, zip, task])
  await chmod(task, 0o755) // Lambda's sandbox user, like this test's, reads /var/task.
  if (arch() !== 'arm64') {
    // The zip carries arm64 Bun. Elsewhere, run the rest of the package on the same release
    // built for this machine (not this process's Bun, which may be patched for Nix).
    execFileSync('python3', [fileURLToPath(new URL('./bun_release.py', import.meta.url)), 'install', join(task, 'bin')], { stdio: 'inherit' })
  }
  api = spawn(process.execPath, [fileURLToPath(new URL('../tests/fixtures/public-api.mjs', import.meta.url))], { stdio: ['ignore', 'pipe', 'inherit'] })
  await new Promise((resolve, reject) => {
    api.stdout.on('data', chunk => { if (String(chunk).includes('fixture API ready')) resolve() })
    api.once('exit', code => reject(new Error(`Fixture API exited with ${code}`)))
  })
  // The function's memory size, and the TZ that Lambda sets for every runtime.
  execFileSync('docker', ['run', '--detach', '--rm', '--name', container, '--network', 'host',
    '--read-only', '--tmpfs', '/tmp', '--user', '993:990', '--volume', `${task}:/var/task:ro`, '--memory', '512m',
    '--env', 'TZ=:UTC', '--env', '_HANDLER=lambda/index.handler', '--env', 'AWS_LAMBDA_FUNCTION_NAME=judge-dev-web',
    '--env', 'AWS_LAMBDA_FUNCTION_MEMORY_SIZE=512', '--env', 'NODE_ENV=production',
    '--env', 'NUXT_API_BASE_URL=http://127.0.0.1:18080', '--env', 'NUXT_PUBLIC_SITE_URL=https://frontend.example',
    '--entrypoint', '/usr/bin/env', image,
    '-u', 'HOME', '/usr/local/bin/aws-lambda-rie', '--runtime-interface-emulator-address', `127.0.0.1:${port}`, '/var/runtime/bootstrap', 'lambda/index.handler'])

  async function invoke(path) {
    const [rawPath, rawQueryString = ''] = path.split('?')
    const event = { version: '2.0', rawPath, rawQueryString, headers: { host: 'frontend.example', 'x-forwarded-proto': 'https' }, requestContext: { http: { method: 'GET', path: rawPath, sourceIp: '203.0.113.50' } }, isBase64Encoded: false }
    for (let attempt = 0; ; attempt++) {
      try {
        const response = await fetch(`http://127.0.0.1:${port}/2015-03-31/functions/function/invocations`, { method: 'POST', body: JSON.stringify(event) })
        const result = await response.json()
        assert.equal(response.status, 200, JSON.stringify(result))
        assert.ok(result.statusCode, `Function error for ${path}: ${JSON.stringify(result)}`)
        return result
      } catch (error) {
        // The emulator needs a moment to listen; any other failure is the function's.
        if (![error?.code, error?.cause?.code].includes('ECONNREFUSED') || attempt >= 50) throw error
        await new Promise(resolve => setTimeout(resolve, 200))
      }
    }
  }
  const body = result => Buffer.from(result.body ?? '', result.isBase64Encoded ? 'base64' : 'utf8')

  const problem = await invoke('/problems/0WCfTognyye34UJm92NqYz')
  assert.equal(problem.statusCode, 200)
  assert.match(body(problem).toString(), /A \+ B/)
  assert.match(body(problem).toString(), /katex-html/)
  const script = (await readdir(join(task, 'public/_nuxt'))).find(file => file.endsWith('.js'))
  const asset = await invoke(`/_nuxt/${script}`)
  assert.equal(asset.statusCode, 200)
  assert.ok(body(asset).length > 0)
  const share = await invoke('/og/docs/markdown.png')
  assert.equal(share.statusCode, 200)
  assert.equal(body(share).subarray(0, 8).toString('hex'), '89504e470d0a1a0a')
  const runtimes = await invoke('/api/runtimes')
  assert.equal(runtimes.statusCode, 200)
  assert.ok(JSON.parse(body(runtimes)).items.length > 0)
  assert.equal((await invoke('/problems/missing')).statusCode, 404)

  const output = containerLogs()
  assert.doesNotMatch(output, /Runtime\.(InvalidEntrypoint|ExitError|ImportModuleError)|Runtime exited/)
  // The emulator starts the runtime on the first invocation: report how long that took.
  console.log(output.match(/INIT REPORT\(durationMs: [\d.]+\)/)?.[0] ?? 'No INIT REPORT', output.match(/REPORT RequestId: \S+ .*?Duration: [\d.]+ ms/)?.[0] ?? '')
  console.log(`${image}: SSR, static files, share image, API proxy, and 404 passed`)
} catch (error) {
  console.error(containerLogs())
  throw error
} finally {
  try { execFileSync('docker', ['rm', '--force', container], { stdio: 'ignore' }) } catch {}
  api?.kill()
  await rm(task, { recursive: true, force: true })
}
