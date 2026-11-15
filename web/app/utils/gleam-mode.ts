import type { StreamParser } from '@codemirror/language'

// Gleam 1.19's keywords; words it only reserves (if, else, auto, ...) are left out.
const keywords = new Set('as assert case const echo fn import let opaque panic pub todo type use'.split(' '))

// ponytail: lexical highlighting; strings may span lines, as in the language.
export const gleam: StreamParser<{ string: boolean }> = {
  name: 'gleam',
  startState: () => ({ string: false }),
  token(stream, state) {
    if (state.string || stream.match('"')) {
      state.string = true
      while (!stream.eol()) {
        const ch = stream.next()
        if (ch === '\\') stream.next()
        else if (ch === '"') { state.string = false; break }
      }
      return 'string'
    }
    if (stream.eatSpace()) return null
    if (stream.match('//')) { stream.skipToEnd(); return 'comment' }
    if (stream.match(/\d\w*(?:\.\d*(?:[eE]-?\d+)?)?/)) return 'number'
    const word = stream.match(/[A-Za-z_]\w*/)
    if (Array.isArray(word)) {
      if (keywords.has(word[0])) return 'keyword'
      if (/^[A-Z]/.test(word[0])) return 'type'
      return 'variable'
    }
    stream.next()
    return null
  },
  languageData: { commentTokens: { line: '//' } },
}
