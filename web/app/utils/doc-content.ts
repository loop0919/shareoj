// Guide pages are Markdown rendered with the same renderer as user content.
// A fence whose info string is `example` (four or more backticks or tildes, so the example may
// contain ordinary fences) shows its source followed by the rendered result.
export type DocBlock = { kind: 'markdown' | 'example', source: string }

export function splitDocExamples(source: string): DocBlock[] {
  const blocks: DocBlock[] = []
  let rest = 0
  for (const match of source.matchAll(/^(`{4,}|~{4,})example[ \t]*\n([\s\S]*?)\n\1[ \t]*$/gm)) {
    if (match.index > rest) blocks.push({ kind: 'markdown', source: source.slice(rest, match.index) })
    blocks.push({ kind: 'example', source: match[2]! })
    rest = match.index + match[0].length
  }
  if (rest < source.length) blocks.push({ kind: 'markdown', source: source.slice(rest) })
  return blocks
}

// Shows Markdown source verbatim in a fence longer than any backtick run inside it.
export function sourceFence(source: string): string {
  const fence = '`'.repeat(Math.max(3, ...[...source.matchAll(/`+/g)].map(run => run[0].length + 1)))
  return `${fence}text\n${source}\n${fence}`
}

// Second-level headings with an explicit {#id} form the page's table of contents.
export function docHeadings(source: string): { id: string, text: string }[] {
  const headings: { id: string, text: string }[] = []
  let fence = ''
  for (const line of source.split('\n')) {
    const marker = /^(`{3,}|~{3,})/.exec(line)?.[1]
    if (fence) {
      if (marker && marker[0] === fence[0] && marker.length >= fence.length && line.trim() === marker) fence = ''
      continue
    }
    if (marker) {
      fence = marker
      continue
    }
    const heading = /^## (.+?)\s+\{#([a-zA-Z][a-zA-Z0-9_-]*)\}\s*$/.exec(line)
    if (heading) headings.push({ id: heading[2]!, text: heading[1]!.replaceAll('`', '') })
  }
  return headings
}
