import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { fileURLToPath } from 'node:url'
import { parseEnv } from 'node:util'
import { startDatabase, localDatabaseURL } from './db.mjs'

const root = fileURLToPath(new URL('../', import.meta.url))
process.chdir(root)
// Keep the one-command startup non-interactive, including Nuxt's first run.
process.env.NUXT_TELEMETRY_DISABLED = '1'

// Shell settings take precedence; never rewrite the user's .env files.
if (existsSync('.env')) {
  for (const [key, value] of Object.entries(parseEnv(readFileSync('.env', 'utf8')))) {
    process.env[key] ??= value
  }
}

const children = new Set()
let stopping = false
function stop(code) {
  if (stopping) return
  stopping = true
  process.exitCode = code
  for (const child of children) {
    try { process.kill(-child.pid, 'SIGTERM') } catch (error) {
      if (error.code !== 'ESRCH') console.error(error.message)
    }
  }
  // Bound shutdown even if a subprocess ignores SIGTERM.
  setTimeout(() => {
    for (const child of children) {
      try { process.kill(-child.pid, 'SIGKILL') } catch {}
    }
  }, 12000).unref()
}
process.on('SIGINT', () => stop(0))
process.on('SIGTERM', () => stop(0))
// Closing the terminal sends SIGHUP; detached servers would otherwise keep their ports.
process.on('SIGHUP', () => stop(0))

function run(command, args, cwd, env = process.env) {
  if (stopping) throw new Error('Startup cancelled')
  const child = spawn(command, args, { cwd, env, stdio: 'inherit', detached: true })
  children.add(child)
  const done = new Promise((resolve, reject) => {
    child.once('error', error => {
      children.delete(child)
      reject(error)
    })
    child.once('exit', (code, signal) => {
      children.delete(child)
      if (code === 0) resolve()
      else reject(new Error(`${command} exited (${signal ?? code})`))
    })
  })
  return done
}

function portNumber(name, fallback) {
  const value = process.env[name] ?? String(fallback)
  if (!/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 65535) {
    throw new Error(`${name} must be an integer between 1 and 65535`)
  }
  return Number(value)
}

async function checkPort(port) {
  await new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', () => reject(new Error(`Port ${port} is unavailable. Set API_PORT / WEB_PORT to use other ports.`)))
    server.listen(port, () => server.close(resolve))
  })
}

async function waitForAPI(port) {
  for (let attempt = 0; attempt < 120 && !stopping; attempt++) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/health`, { signal: AbortSignal.timeout(1000) })
      if (response.ok) return
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error('API did not become ready')
}

try {
  const apiPort = portNumber('API_PORT', 8080)
  const webPort = portNumber('WEB_PORT', 3000)
  if (apiPort === webPort) throw new Error('API_PORT and WEB_PORT must differ')
  await checkPort(apiPort)
  await checkPort(webPort)

  if (!process.env.DATABASE_URL) {
    startDatabase()
    process.env.DATABASE_URL = localDatabaseURL
    await run('go', ['run', './cmd/migrate'], `${root}api`)
  }

  const fingerprint = createHash('sha256')
    .update(readFileSync('web/package.json'))
    .update(readFileSync('web/package-lock.json'))
    .update(process.version)
    .digest('hex')
  const stamp = 'web/node_modules/.dev-dependencies'
  if (!existsSync(stamp) || readFileSync(stamp, 'utf8') !== fingerprint) {
    console.log('Installing frontend dependencies…')
    await run('npm', ['ci'], `${root}web`)
    writeFileSync(stamp, fingerprint)
  }

  mkdirSync('api/.build', { recursive: true })
  console.log('Building API…')
  await run('go', ['build', '-o', '.build/local-api', './cmd/api'], `${root}api`)
  const localJudge = Boolean(process.env.JUDGE_CPP_IMAGE)
  if (localJudge) await run('go', ['build', '-o', '.build/judge-worker', './cmd/judge-worker'], `${root}api`)
  const supervise = promise => promise.then(() => stop(1), error => {
    if (!stopping) console.error(error.message)
    stop(1)
  })
  if (localJudge) supervise(run(`${root}api/.build/judge-worker`, [], `${root}api`))
  supervise(run(`${root}api/.build/local-api`, [], `${root}api`, {
    ...process.env, PORT: String(apiPort),
  }))
  await waitForAPI(apiPort)
  if (!stopping) {
    console.log(`\nWeb: http://localhost:${webPort}/\nAPI: http://localhost:${apiPort}/health\nStop both servers with Ctrl+C.\n`)
    supervise(run(process.execPath, ['node_modules/nuxt/bin/nuxt.mjs', 'dev', '--host', '127.0.0.1', '--port', String(webPort)], `${root}web`, {
      ...process.env,
      PORT: String(webPort),
      NUXT_API_BASE_URL: `http://127.0.0.1:${apiPort}`,
      NUXT_PUBLIC_SITE_URL: `http://localhost:${webPort}`,
    }))
  }
} catch (error) {
  if (!stopping) console.error(error.message)
  stop(1)
}
