import type { StreamParser } from '@codemirror/language'

// From the upstream VS Code grammar (vscode/syntaxes/goose.tmLanguage.json, v2026.3).
const keywords = new Set('if else while for in loop block guard match return break continue import namespace as struct enum type fn thread_fn let var const export extern simd recursive pad reusable'.split(' '))
const types = new Set('bool varint i8 i16 i32 i64 u8 u16 u32 u64 f32 f64'.split(' '))

// ponytail: lexical highlighting; block comments nest as in the language.
export const goose: StreamParser<{ comments: number, triple: boolean }> = {
  name: 'goose',
  startState: () => ({ comments: 0, triple: false }),
  token(stream, state) {
    if (state.comments || stream.match('/*')) {
      if (!state.comments) state.comments = 1
      while (!stream.eol()) {
        if (stream.match('/*')) state.comments++
        else if (stream.match('*/')) { if (!--state.comments) break }
        else stream.next()
      }
      return 'comment'
    }
    if (state.triple || stream.match('"""')) {
      state.triple = true
      while (!stream.eol()) {
        if (stream.match('"""')) { state.triple = false; break }
        stream.next()
      }
      return 'string'
    }
    if (stream.eatSpace()) return null
    if (stream.match('//')) { stream.skipToEnd(); return 'comment' }
    if (stream.match(/"(?:[^"\\]|\\.)*"?|'(?:[^'\\]|\\.)*'?/)) return 'string'
    if (stream.match(/\d\w*(?:\.\d\w*)?/)) return 'number'
    const word = stream.match(/[A-Za-z_]\w*/)
    if (Array.isArray(word)) {
      if (keywords.has(word[0])) return 'keyword'
      if (types.has(word[0])) return 'type'
      if (['true', 'false', 'null'].includes(word[0])) return 'atom'
      return 'variable'
    }
    stream.next()
    return null
  },
  languageData: { commentTokens: { line: '//', block: { open: '/*', close: '*/' } } },
}
