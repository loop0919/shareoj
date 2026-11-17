import type { StreamParser } from '@codemirror/language'

// Keywords and pragmas of Fix 1.6's grammar.
const keywords = new Set('module import hiding namespace let in if else match do eval type struct union box unbox trait impl FFI_CALL FFI_CALL_IO FFI_CALL_IOS FFI_EXPORT DEPRECATED'.split(' '))

// ponytail: lexical highlighting; capitalised names are types or namespaces.
export const fix: StreamParser<{ comment: boolean }> = {
  name: 'fix',
  startState: () => ({ comment: false }),
  token(stream, state) {
    if (state.comment || stream.match('/*')) {
      state.comment = true
      while (!stream.eol()) {
        if (stream.match('*/')) { state.comment = false; break }
        stream.next()
      }
      return 'comment'
    }
    if (stream.eatSpace()) return null
    if (stream.match('//')) { stream.skipToEnd(); return 'comment' }
    if (stream.match(/"(?:[^"\\]|\\.)*"?|'(?:[^'\\]|\\.)*'?/)) return 'string'
    if (stream.match(/\d\w*(?:\.\d\w*)?/)) return 'number'
    const word = stream.match(/[A-Za-z_]\w*/)
    if (Array.isArray(word)) {
      if (keywords.has(word[0])) return 'keyword'
      if (['true', 'false'].includes(word[0])) return 'atom'
      if (/^[A-Z]/.test(word[0])) return 'type'
      return 'variable'
    }
    stream.next()
    return null
  },
  languageData: { commentTokens: { line: '//', block: { open: '/*', close: '*/' } } },
}
