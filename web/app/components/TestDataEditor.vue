<script setup lang="ts">
import { basicSetup } from 'codemirror'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { testFileLimit } from '~~/shared/types/problem-draft'

const text = defineModel<string>({ required: true })
const props = defineProps<{ id: string, label: string, disabled: boolean, storedBytes?: number, loading?: boolean, error?: string }>()
const container = ref<HTMLDivElement>()
const file = ref<HTMLInputElement>()
const fileError = ref('')
const encoder = new TextEncoder()
const bytes = ref(text.value ? encoder.encode(text.value).length : (props.storedBytes ?? 0))
const editable = new Compartment()
const attributes = new Compartment()
let editor: EditorView | undefined
const themeExtension = useCodeMirrorTheme(() => editor)
let fromModel = false
let emittedText: string | undefined
const editing = () => [EditorState.readOnly.of(props.disabled), EditorView.editable.of(!props.disabled)]
const contentAttributes = () => EditorView.contentAttributes.of({ id: props.id, 'aria-labelledby': `${props.id}-label`, 'aria-multiline': 'true', ...(props.disabled ? { 'aria-disabled': 'true' } : {}) })
async function selectFile(event: Event) {
  const selected = (event.target as HTMLInputElement).files?.[0]
  if (!selected) return
  fileError.value = ''
  try {
    if (selected.size > testFileLimit) throw new Error('16 MiBを超えるファイルは選択できません。')
    const value = new TextDecoder('utf-8', { fatal: true }).decode(await selected.arrayBuffer())
    if (value.includes('\0')) throw new Error('NUL文字を含むファイルは選択できません。')
    text.value = value
  } catch (error) {
    fileError.value = error instanceof TypeError ? 'UTF-8のテキストファイルを選択してください。' : (error as Error).message
  } finally {
    if (file.value) file.value.value = ''
  }
}

onMounted(() => {
  editor = new EditorView({
    parent: container.value,
    doc: text.value,
    extensions: [
      basicSetup,
      themeExtension(),
      editable.of(editing()),
      attributes.of(contentAttributes()),
      EditorView.updateListener.of((update) => {
        if (!update.docChanged || fromModel) return
        update.changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
          bytes.value += encoder.encode(inserted.toString()).length - encoder.encode(update.startState.sliceDoc(fromA, toA)).length
        })
        emittedText = update.state.doc.toString()
        text.value = emittedText
      }),
      EditorView.theme({
        '&': { height: '100%', color: 'var(--color-ink)', backgroundColor: 'transparent' },
        '&.cm-focused': { outline: 'none' },
        '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-code)', fontSize: '14px', lineHeight: '22px', overscrollBehavior: 'contain' },
        '.cm-content': { padding: '0' },
        '.cm-line': { padding: '0 16px' },
        '.cm-gutters': { backgroundColor: 'transparent', color: 'var(--color-muted)', borderRight: '1px solid var(--color-line)' },
        '.cm-lineNumbers .cm-gutterElement': { minWidth: '48px', paddingRight: '10px' },
        '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--color-accent-soft)' },
      }),
    ],
  })
})
watch(() => props.disabled, () => editor?.dispatch({ effects: [editable.reconfigure(editing()), attributes.reconfigure(contentAttributes())] }))
watch(() => props.id, () => {
  editor?.dispatch({ effects: attributes.reconfigure(contentAttributes()) })
  if (!text.value) bytes.value = props.storedBytes ?? 0
})
watch(text, (value) => {
  if (!editor) return
  if (value === emittedText) { emittedText = undefined; return }
  bytes.value = value ? encoder.encode(value).length : (props.storedBytes ?? 0)
  const current = editor.state.doc.toString()
  fromModel = true
  // Saving appends the final newline; inserting only that keeps the cursor before it while the author types.
  editor.dispatch({ changes: value.startsWith(current) ? { from: current.length, insert: value.slice(current.length) } : { from: 0, to: current.length, insert: value } })
  fromModel = false
}, { flush: 'sync' })
watch(() => props.storedBytes, value => {
  if (!text.value) bytes.value = value ?? 0
})
onBeforeUnmount(() => editor?.destroy())
</script>

<template>
  <div class="test-data-editor">
    <div class="pane-heading"><label :id="`${id}-label`" :for="id">{{ label }}</label><span>{{ bytes.toLocaleString('en-US') }} / 16,777,216 bytes</span><button type="button" class="editor-button" :disabled="disabled || loading" @click="file?.click()">ファイルを選択</button><input ref="file" hidden type="file" accept=".txt,text/plain" :disabled="disabled || loading" @change="selectFile"></div>
    <p v-if="loading" class="file-status"><LoadingSpinner inline label="ファイルを読み込み中" /></p>
    <p v-else-if="error || fileError" class="file-status field-error" role="alert">{{ error || fileError }}</p>
    <div ref="container" class="code-surface" />
  </div>
</template>

<style scoped>
.test-data-editor { display: flex; flex-direction: column; min-width: 0; min-height: 0; border-right: 1px solid var(--color-line); }
.pane-heading { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--color-line); }
.pane-heading label { margin: 0; font-size: .875rem; }
.pane-heading span { margin-left: auto; font-size: .75rem; color: var(--color-muted); }
.pane-heading button { padding: 3px 7px; font-size: .6875rem; white-space: nowrap; }
.file-status { margin: 0; padding: 4px 12px; font-size: .75rem; border-bottom: 1px solid var(--color-line); }
.code-surface { position: relative; flex: 1; min-width: 0; min-height: 0; overflow: hidden; }
.code-surface:focus-within::after { content: ""; position: absolute; inset: 0; z-index: 10; border: 2px solid var(--color-accent); pointer-events: none; }
</style>
