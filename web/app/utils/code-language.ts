import { cpp } from '@codemirror/lang-cpp'
import { StreamLanguage } from '@codemirror/language'
import { c, java, csharp } from '@codemirror/legacy-modes/mode/clike'
import { python } from '@codemirror/legacy-modes/mode/python'
import { rust } from '@codemirror/legacy-modes/mode/rust'
import { nim } from './nim-mode'
import { go } from '@codemirror/legacy-modes/mode/go'
import { haskell } from '@codemirror/legacy-modes/mode/haskell'
import { javascript, typescript } from '@codemirror/legacy-modes/mode/javascript'
import { ruby } from '@codemirror/legacy-modes/mode/ruby'
import { Prec } from '@codemirror/state'
import { keymap } from '@codemirror/view'
import { insertNewline, insertTab } from '@codemirror/commands'

const modes = { c, java, csharp, python, rust, nim, go, haskell, javascript, typescript, ruby }

export function codeLanguage(runtime: string): string {
  if (runtime.startsWith('clay')) return 'cpp'
  if (runtime.startsWith('whitespace')) return 'whitespace'
  if (/^(python|pypy|codon)/.test(runtime)) return 'python'
  return runtime.match(/^(cpp|csharp|javascript|typescript|java|rust|nim|go|haskell|ruby|c)(?=\d|-|$)/)?.[1] ?? ''
}

export function codeLanguageSupport(language: string) {
  if (language === 'cpp') return cpp()
  if (language === 'whitespace') return Prec.highest(keymap.of([
    { key: 'Tab', run: insertTab }, { key: 'Enter', run: insertNewline },
  ]))
  const mode = modes[language as keyof typeof modes]
  return mode ? StreamLanguage.define(mode) : []
}
