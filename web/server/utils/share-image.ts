import { initWasm, Resvg } from '@resvg/resvg-wasm'

let ready: Promise<Uint8Array> | undefined
function loadRenderer() {
  return ready ??= (async () => {
    const wasm = await useStorage('assets:resvg').getItemRaw<Buffer>('index_bg.wasm')
    const font = await useStorage('assets:server').getItemRaw<Buffer>('fonts/ipaexg.ttf')
    if (!wasm || !font) throw new Error('Share image assets are missing')
    await initWasm(wasm)
    return new Uint8Array(font)
  })().catch(error => { ready = undefined; throw error })
}

export function shareImageSvg(title: string, kind: string) {
  const escape = (text: string) => text.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[char]!)
  const graphemes = new Intl.Segmenter('ja', { granularity: 'grapheme' })
  const width = (text: string) => [...graphemes.segment(text)].reduce((sum, { segment }) => sum + (segment === ' ' ? 0.35 : /^[\x00-\x7f]$/.test(segment) ? 0.55 : 1), 0)
  const lines: string[] = []
  let line = ''
  const cleanTitle = title.replace(/[\p{Cc}\p{Cf}]/gu, ' ').replace(/\s+/g, ' ').trim()
  for (const chunk of cleanTitle.split(/( )/)) {
    const segments = width(chunk) <= 17 ? [chunk] : [...new Intl.Segmenter('ja', { granularity: 'word' }).segment(chunk)].map(item => item.segment)
    for (const segment of segments) {
      let rest = segment
      while (rest) {
        const next = (line + rest).trimStart()
        if (width(next) <= 17) { line = next; break }
        if (line) { lines.push(line.trimEnd()); line = ''; rest = rest.trimStart(); continue }
        const chars = [...graphemes.segment(rest)].map(item => item.segment)
        let end = 1
        while (end < chars.length && width(chars.slice(0, end + 1).join('')) <= 17) end++
        lines.push(chars.slice(0, end).join(''))
        rest = chars.slice(end).join('')
      }
    }
  }
  if (line) lines.push(line.trimEnd())
  if (lines.length > 3) {
    lines.length = 3
    while (width(lines[2] + '…') > 17) lines[2] = [...graphemes.segment(lines[2]!)].slice(0, -1).map(item => item.segment).join('').trimEnd()
    lines[2] += '…'
  }
  for (let i = lines.length - 2; i >= 0; i--) {
    let cut: number
    while ((cut = lines[i]!.lastIndexOf(' ')) > 0) {
      const first = lines[i]!.slice(0, cut)
      const second = `${lines[i]!.slice(cut + 1)} ${lines[i + 1]}`
      if (width(second) > 17 || Math.abs(width(first) - width(second)) >= Math.abs(width(lines[i]!) - width(lines[i + 1]!))) break
      lines[i] = first
      lines[i + 1] = second
    }
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
    <rect width="1200" height="630" fill="#fff"/>
    <rect width="16" height="630" fill="#18794e"/>
    <g transform="translate(72 56)">
      <rect width="64" height="64" rx="12" fill="#192431"/>
      <g stroke="#fff" stroke-width="5" fill="none"><path d="m15 22-7 10 7 10m34-20 7 10-7 10"/></g>
      <path d="m22 33 8 8 12-18" stroke="#57e3a0" stroke-width="5" fill="none"/>
    </g>
    <g font-family="IPAexGothic" fill="#192431">
      <text x="156" y="102" font-size="40">Share<tspan fill="#18794e">OJ</tspan></text>
      <text x="1128" y="96" text-anchor="end" font-size="24" fill="#536171">${escape(kind)}</text>
      ${lines.map((line, i) => `<text x="72" y="${lines.length === 1 ? 324 : 260 + i * 82}" font-size="60">${escape(line)}</text>`).join('')}
      <path d="M72 514H1128" stroke="#dce2e8"/>
      <text x="72" y="574" font-size="24" fill="#536171">問題をつくる。解く。共有する。</text>
      <text x="1128" y="574" text-anchor="end" font-size="26" fill="#18794e">#ShareOJ</text>
    </g>
  </svg>`
}

export async function renderShareImage(title: string, kind: string) {
  const font = await loadRenderer()
  const renderer = new Resvg(shareImageSvg(title, kind), { font: { fontBuffers: [font], defaultFontFamily: 'IPAexGothic', loadSystemFonts: false } })
  try {
    const image = renderer.render()
    try { return Buffer.from(image.asPng()) }
    finally { image.free() }
  } finally { renderer.free() }
}
