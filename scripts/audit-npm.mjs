// Fail on high or critical advisories in production npm dependencies, except
// reviewed ones that the allowlist names with a reason and a review date.
import { spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

const [directory, allowlistPath] = process.argv.slice(2)
if (!directory || !allowlistPath) {
  console.error('usage: bun scripts/audit-npm.mjs <package directory> <allowlist.json>')
  process.exit(2)
}
// bun audit reads bun.lock, asks the npm registry, and exits non-zero whenever it finds
// anything, so judge by its JSON report: package name -> advisories.
const audit = spawnSync(process.execPath, ['audit', '--omit=dev', '--json'], { cwd: directory, encoding: 'utf8', maxBuffer: 64 << 20 })
let report
try { report = JSON.parse(audit.stdout) }
catch { console.error(audit.stderr || audit.stdout); process.exit(2) }
if (!report || typeof report !== 'object' || !Object.values(report).every(Array.isArray)) {
  console.error(audit.stderr || audit.stdout)
  process.exit(2)
}

const advisories = new Map()
for (const [name, entries] of Object.entries(report)) {
  for (const advisory of entries) {
    if (!['high', 'critical'].includes(advisory.severity)) continue
    const id = advisory.url.split('/').pop()
    advisories.set(id, { severity: advisory.severity, line: `${advisory.severity} ${name}: ${advisory.title} ${advisory.url}` })
  }
}

const today = new Date().toISOString().slice(0, 10)
const allowed = new Map(JSON.parse(readFileSync(allowlistPath, 'utf8')).map(entry => [entry.id, entry]))
let failed = false
for (const [id, advisory] of advisories) {
  const entry = allowed.get(id)
  if (!entry) {
    console.log(`::error::${advisory.line}`)
    failed = true
  }
  else if (entry.until < today) {
    console.log(`::error::Review expired on ${entry.until}: ${advisory.line}`)
    failed = true
  }
  else console.log(`Allowed until ${entry.until} (${entry.reason}): ${advisory.line}`)
}
for (const id of allowed.keys()) {
  if (!advisories.has(id)) console.log(`::warning::${id} is no longer reported; remove it from ${allowlistPath}`)
}
console.log(`${advisories.size} high or critical advisories; ${failed ? 'some are not reviewed' : 'all are reviewed'}`)
process.exit(failed ? 1 : 0)
