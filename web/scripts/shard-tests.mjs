// Split a Playwright config's tests into CI shards that take about the same time.
// usage: bun scripts/shard-tests.mjs split <config> <shards> <directory>
//          writes <directory>/<n>.txt, n = 1 … shards, for `playwright test --test-list`
//        bun scripts/shard-tests.mjs record <report.json>…
//          records the durations of JSON reports in tests/durations.json, the fastest run of a
//          test when several reports have it; make a report with
//          PLAYWRIGHT_JSON_OUTPUT_FILE=<report.json> bun run test --reporter=json
// Playwright's own --shard gives each shard the same number of tests, so a shard of slow tests
// ran twice as long as another. A test without a recorded duration counts as the median of the
// others: new tests still run, and old durations only make the shards less even.
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { basename, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const web = fileURLToPath(new URL('..', import.meta.url))
const durationsFile = join(web, 'tests/durations.json')

// How --test-list names a test: its file, then its describe titles and its own title.
export const line = test => [test.file, ...test.titles].join(' › ')

function walk(report, visit) {
  const visitSuite = (suite, file, titles) => {
    for (const spec of suite.specs ?? []) visit({ file, titles: [...titles, spec.title] }, spec)
    for (const child of suite.suites ?? []) visitSuite(child, file, [...titles, child.title])
  }
  for (const suite of report.suites ?? []) visitSuite(suite, suite.file, [])
}

export function testsOf(report) {
  const tests = []
  walk(report, test => {
    const title = test.titles.find(title => title.includes('›'))
    if (title !== undefined) throw new Error(`${test.file}: --test-list would split the title ${JSON.stringify(title)} at its ›`)
    tests.push(test)
  })
  return tests
}

export function durationsOf(report) {
  const durations = {}
  walk(report, (test, spec) => {
    const attempts = (spec.tests ?? []).map(run => run.results?.at(-1)).filter(Boolean)
    if (attempts.length) durations[line(test)] = Math.round(attempts.reduce((sum, result) => sum + result.duration, 0) / 100) / 10
  })
  return durations
}

export function split(tests, durations, shards) {
  const known = tests.map(test => durations[line(test)]).filter(value => value !== undefined).sort((a, b) => a - b)
  const median = known.length ? (known[Math.floor((known.length - 1) / 2)] + known[Math.ceil((known.length - 1) / 2)]) / 2 : 1
  // A test recorded at zero (skipped, or no page) still takes a moment, and must add to its shard.
  const weight = test => Math.max(durations[line(test)] ?? median, 0.1)
  // Longest first, each to the shard with the least time so far; the sort keeps ties in list order.
  const loads = Array(shards).fill(0)
  const shardOf = new Map()
  for (const test of [...tests].sort((a, b) => weight(b) - weight(a))) {
    const shard = loads.indexOf(Math.min(...loads))
    loads[shard] += weight(test)
    shardOf.set(test, shard)
  }
  return loads.map((_, shard) => tests.filter(test => shardOf.get(test) === shard))
}

// Durations from reports replace the recorded ones of their config. A test in several reports, such
// as the shards of more than one run, keeps its fastest run: a slow runner slows every test on it.
export function merge(recorded, reports) {
  const fresh = {}
  for (const report of reports) {
    const config = basename(report.config.configFile)
    fresh[config] ??= {}
    for (const [key, seconds] of Object.entries(durationsOf(report))) fresh[config][key] = Math.min(fresh[config][key] ?? Infinity, seconds)
  }
  const merged = { ...recorded }
  for (const [config, durations] of Object.entries(fresh)) {
    const all = { ...recorded[config], ...durations }
    merged[config] = Object.fromEntries(Object.keys(all).sort().map(key => [key, all[key]]))
  }
  return merged
}

const readDurations = () => existsSync(durationsFile) ? JSON.parse(readFileSync(durationsFile, 'utf8')) : {}

if (import.meta.main) {
  const [command, ...args] = process.argv.slice(2)
  if (command === 'split' && args.length === 3) {
    const [config, count, directory] = args
    const list = execFileSync(process.execPath, ['--bun', 'playwright', 'test', '--config', config, '--list', '--reporter=json'], {
      cwd: web, encoding: 'utf8', maxBuffer: 1 << 26,
      // The account config wants a database before it loads; listing never connects to it.
      env: { ...process.env, TEST_DATABASE_URL: process.env.TEST_DATABASE_URL || 'postgresql://listing-only' },
    })
    const tests = testsOf(JSON.parse(list))
    const durations = readDurations()[basename(config)] ?? {}
    const shards = split(tests, durations, Number(count))
    if (shards.some(shard => !shard.length)) throw new Error(`${config} has ${tests.length} tests, too few for ${count} shards`)
    mkdirSync(directory, { recursive: true })
    shards.forEach((shard, i) => writeFileSync(join(directory, `${i + 1}.txt`), `${shard.map(line).join('\n')}\n`))
    const seconds = shard => Math.round(shard.reduce((sum, test) => sum + (durations[line(test)] ?? 0), 0))
    console.log(`${config}: ${tests.length} tests, ${tests.filter(test => durations[line(test)] === undefined).length} without a duration; recorded seconds per shard: ${shards.map(seconds).join(' ')}`)
  } else if (command === 'record' && args.length) {
    const reports = args.map(path => JSON.parse(readFileSync(path, 'utf8')))
    writeFileSync(durationsFile, `${JSON.stringify(merge(readDurations(), reports), null, 2)}\n`)
    console.log(`${reports.length} reports recorded in tests/durations.json`)
  } else {
    console.error('usage: bun scripts/shard-tests.mjs split <config> <shards> <directory> | record <report.json>…')
    process.exit(2)
  }
}
