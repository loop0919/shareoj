import katex from 'katex'

export type MathTextPart =
  | { kind: 'text'; text: string }
  | { kind: 'math'; html: string; display: boolean }

// Reject non-LaTeX input, but let \\ break lines in display math. LaTeX ignores it there,
// and KaTeX copies that under strict mode, so a strict 'error' would drop every line break.
export function katexStrictness(code: string) {
  return code === 'newLineInDisplayMode' ? 'ignore' as const : 'error' as const
}

// Dollar delimiters only; ordinary prose is always rendered as Vue text.
export function renderMathText(source: string): MathTextPart[] {
  const parts: MathTextPart[] = []
  let text = ''
  let offset = 0
  const flushText = () => {
    if (text) parts.push({ kind: 'text', text })
    text = ''
  }

  while (offset < source.length) {
    if (source[offset] === '\\' && source[offset + 1] === '$') {
      text += '$'
      offset += 2
      continue
    }
    if (source[offset] === '\\' && source[offset + 1] === '\\') {
      text += '\\\\'
      offset += 2
      continue
    }
    if (source[offset] !== '$') {
      text += source[offset++]
      continue
    }

    const delimiter = source.startsWith('$$', offset) ? '$$' : '$'
    const start = offset + delimiter.length
    let end = start
    let braces = 0
    while (end < source.length) {
      if (source[end] === '\\') {
        end += 2
        continue
      }
      if (braces === 0 && source.startsWith(delimiter, end)) break
      if (source[end] === '{') braces++
      if (source[end] === '}') braces = Math.max(0, braces - 1)
      end++
    }
    if (end >= source.length) {
      // Preserve incomplete expressions verbatim while an author edits them.
      text += source.slice(offset)
      break
    }

    const expression = source.slice(start, end)
    const raw = source.slice(offset, end + delimiter.length)
    try {
      if (expression.length > 10_000) throw new Error('Expression too long')
      const html = katex.renderToString(expression, {
        displayMode: delimiter === '$$',
        output: 'htmlAndMathml',
        throwOnError: true,
        trust: false,
        strict: katexStrictness,
        maxExpand: 1000,
        maxSize: 20,
      })
      flushText()
      parts.push({ kind: 'math', html, display: delimiter === '$$' })
    } catch {
      text += raw
    }
    offset = end + delimiter.length
  }
  flushText()
  return parts
}
