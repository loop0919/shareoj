// Samples in a statement, kept in step with the test cases (issue #18). A sample with a name sits between
//   <!-- sample-begin: NAME -->  and  <!-- sample-end: NAME -->
// and everything between the two is rewritten from the test case; notes on a sample go after the end marker.
// cli/samples.go implements the same rules, and cli/testdata/sample-sync.json holds the cases both must pass.
export interface SampleText { name?: string, input: string, output: string }

const startMarker = /<!-- sample-begin: (.+?) -->/g
const heading = /^## サンプル (\d{1,6})\s*$/m
const endMarker = (name: string) => `<!-- sample-end: ${name} -->`

// A name that can sit in a marker and be found again.
export const syncableSampleName = (name?: string): name is string => !!name && name.trim() === name && !/-->|[\r\n]/.test(name)

function fenced(value: string) {
  let length = 3
  for (const match of value.matchAll(/`+/g)) length = Math.max(length, match[0].length + 1)
  const fence = '`'.repeat(length)
  return `${fence}text\n${value}${value && !value.endsWith('\n') ? '\n' : ''}${fence}`
}
const body = (number: number, sample: SampleText) => `## サンプル ${number}\n\n### 入力\n${fenced(sample.input)}\n\n### 出力\n${fenced(sample.output)}\n`

// Rewrites the marked samples in place and appends the rest in order. A marked sample keeps its number; a sample
// whose name cannot sit in a marker is appended bare, so it is appended again by the next call. A marker that names
// no sample is left alone.
export function syncSamples(text: string, samples: SampleText[]) {
  let next = 0
  for (const match of text.matchAll(new RegExp(heading.source, 'gm'))) next = Math.max(next, Number(match[1]))
  const named = new Map(samples.filter(sample => syncableSampleName(sample.name)).map(sample => [sample.name!, sample]))
  const seen = new Set<string>()
  let out = '', position = 0, updated = 0
  for (const match of text.matchAll(startMarker)) {
    const name = match[1]!, from = match.index + match[0].length
    const sample = named.get(name)
    const to = text.indexOf(endMarker(name), from)
    if (from < position || !sample || to < 0) continue
    const inner = text.slice(from, to)
    const number = heading.exec(inner)
    const fresh = `\n\n${body(number ? Number(number[1]) : ++next, sample)}\n`
    if (fresh !== inner) updated++
    out += text.slice(position, from) + fresh
    position = to
    seen.add(name)
  }
  out += text.slice(position)
  const blocks: string[] = []
  for (const sample of samples) {
    const name = syncableSampleName(sample.name) ? sample.name : undefined
    if (name !== undefined && seen.has(name)) continue
    const block = body(++next, sample)
    blocks.push(name === undefined ? block : `<!-- sample-begin: ${name} -->\n\n${block}\n${endMarker(name)}\n`)
    if (name !== undefined) seen.add(name)
  }
  if (blocks.length) out += (out ? '\n\n' : '') + blocks.join('\n')
  return { text: out, updated, added: blocks.length }
}
