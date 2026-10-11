<script setup lang="ts">
import { basicSetup } from 'codemirror'
import { Compartment, EditorState, Prec, Transaction } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { indentWithTab } from '@codemirror/commands'
import { defaultHighlightStyle, HighlightStyle, indentUnit, syntaxHighlighting } from '@codemirror/language'
import { markdown, markdownLanguage, insertNewlineContinueMarkupCommand, deleteMarkupBackward } from '@codemirror/lang-markdown'
import { contentImageError, prepareContentImage } from '~/utils/content-image'

defineOptions({ inheritAttrs: false })
const source = defineModel<string>({ required: true })
const props = defineProps<{ id: string, labelId: string, disabled?: boolean, invalid?: boolean, describedby?: string }>()
const emit = defineEmits<{ blur: [], scroll: [] }>()
const container = ref<HTMLDivElement>()
const fileInput = ref<HTMLInputElement>()
const uploading = ref(false)
const imageMessage = ref('')
const showImages = ref(false)
let uploadPosition: number | undefined
let uploadGeneration = 0
const { settings, indentation } = useEditorSettings('markdown')
const indentConfig = new Compartment()
const editable = new Compartment()
let editor: EditorView | undefined
const themeExtension = useCodeMirrorTheme(() => editor)
const indentExtensions = () => [indentUnit.of(indentation.value), EditorState.tabSize.of(settings.value.width)]
const editing = () => [
  EditorState.readOnly.of(Boolean(props.disabled)),
  EditorView.editable.of(!props.disabled),
  EditorView.contentAttributes.of({
    id: props.id, 'aria-labelledby': props.labelId, 'aria-multiline': 'true',
    'aria-disabled': String(Boolean(props.disabled)), 'aria-invalid': String(Boolean(props.invalid)),
    ...(props.describedby ? { 'aria-describedby': props.describedby } : {}),
    spellcheck: 'false',
  }),
]

onMounted(() => {
  editor = new EditorView({
    parent: container.value,
    doc: source.value,
    extensions: [
      basicSetup,
      themeExtension(),
      markdown({ base: markdownLanguage, addKeymap: false }),
      Prec.high(keymap.of([
        { key: 'Enter', run: insertNewlineContinueMarkupCommand({ nonTightLists: false }) },
        { key: 'Backspace', run: deleteMarkupBackward },
        indentWithTab,
      ])),
      syntaxHighlighting(HighlightStyle.define(defaultHighlightStyle.specs.map(style => ({
        ...style,
        ...(style.color ? { color: `light-dark(${style.color}, color-mix(in srgb, ${style.color} 35%, white))` } : {}),
      })))),
      indentConfig.of(indentExtensions()),
      editable.of(editing()),
      EditorView.lineWrapping,
      EditorState.transactionFilter.of(transaction =>
        transaction.docChanged && transaction.newDoc.length > 100_000 && transaction.newDoc.length > transaction.startState.doc.length ? [] : transaction),
      EditorView.updateListener.of(update => {
        if (uploadPosition !== undefined) uploadPosition = update.changes.mapPos(uploadPosition, 1)
        if (update.docChanged) source.value = update.state.doc.toString()
      }),
      EditorView.domEventHandlers({
        blur: () => { emit('blur') },
        dragover: event => {
          if (!event.dataTransfer?.types.includes('Files')) return false
          event.preventDefault()
          event.dataTransfer.dropEffect = props.disabled || uploading.value ? 'none' : 'copy'
          return true
        },
        drop: (event, view) => {
          if (!event.dataTransfer?.files.length) return false
          event.preventDefault()
          const position = view.posAtCoords({ x: event.clientX, y: event.clientY }) ?? view.state.selection.main.head
          void uploadImages(Array.from(event.dataTransfer.files), position)
          return true
        },
        paste: event => {
          const files = Array.from(event.clipboardData?.files ?? [])
          if (!files.length) return false
          event.preventDefault()
          void uploadImages(files)
          return true
        },
      }),
      EditorView.theme({
        '&': { height: '100%', color: 'var(--color-ink)', backgroundColor: 'var(--color-paper)' },
        '&.cm-focused': { outline: 'none' },
        '.cm-scroller': { overflow: 'auto', overscrollBehavior: 'contain', fontFamily: 'var(--font-code)', fontSize: '.875rem', lineHeight: '22px' },
        '.cm-content': { padding: '0', minHeight: '100%' },
        '.cm-line': { padding: '0 16px' },
        '.cm-gutters': { backgroundColor: 'var(--color-paper)', color: 'var(--color-muted)', borderRight: '1px solid var(--color-line)' },
        '.cm-lineNumbers .cm-gutterElement': { minWidth: '48px', paddingRight: '12px' },
        '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--color-accent-soft)' },
        '@media (max-width: 39.999rem)': { '.cm-scroller': { fontSize: '1rem' } },
      }),
    ],
  })
  editor.scrollDOM.addEventListener('scroll', () => emit('scroll'), { passive: true })
})
watch(() => [props.disabled, props.invalid, props.describedby], () => editor?.dispatch({ effects: editable.reconfigure(editing()) }))
watch(() => [settings.value.style, settings.value.width], () => editor?.dispatch({ effects: indentConfig.reconfigure(indentExtensions()) }))
watch(source, value => {
  if (editor && value !== editor.state.doc.toString()) {
    uploadGeneration++
    uploadPosition = undefined
    editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value }, annotations: Transaction.addToHistory.of(false), filter: false })
  }
})
onBeforeUnmount(() => { uploadGeneration++; editor?.destroy(); editor = undefined })

async function uploadImages(files: File[], position = editor?.state.selection.main.head) {
  if (!editor || props.disabled || uploading.value || position === undefined) return
  if (files.length > 10) { imageMessage.value = '一度に追加できる画像は10枚までです。'; return }
  uploading.value = true
  imageMessage.value = ''
  showImages.value = false
  uploadPosition = position
  const generation = uploadGeneration
  try {
    for (const file of files) {
      const data = await prepareContentImage(file)
      if (generation !== uploadGeneration || props.disabled) break
      const result = await $fetch<{ url: string }>('/api/my/images', { method: 'POST', body: { data } })
      if (!editor || generation !== uploadGeneration || props.disabled || uploadPosition === undefined) break
      const snippet = `![画像の説明](${result.url})\n`
      if (editor.state.doc.length + snippet.length > 100_000) {
        throw new Error('本文の文字数上限に達しました。画像は「保存した画像」から再挿入できます。')
      }
      editor.dispatch({ changes: { from: uploadPosition, insert: snippet }, userEvent: 'input' })
    }
    editor?.focus()
  } catch (error) { imageMessage.value = contentImageError(error) }
  finally { uploading.value = false; uploadPosition = undefined }
}

function chooseImages(event: Event) {
  const input = event.target as HTMLInputElement
  void uploadImages(Array.from(input.files ?? []))
  input.value = ''
}

// Scroll sync measures heights from the top of the scrolled content and lines as fractional 0-based numbers.
function contentTop(view: EditorView) {
  return view.documentTop - view.scrollDOM.getBoundingClientRect().top + view.scrollDOM.scrollTop
}
function blockLines(view: EditorView, block: { from: number, to: number }) {
  return [view.state.doc.lineAt(block.from).number - 1, view.state.doc.lineAt(block.to).number] as const
}
function lineAtHeight(height: number) {
  if (!editor) return 0
  const offset = height - contentTop(editor)
  const block = editor.lineBlockAtHeight(offset)
  const [first, last] = blockLines(editor, block)
  return first + (last - first) * Math.min(1, Math.max(0, (offset - block.top) / (block.height || 1)))
}
function heightAtLine(line: number) {
  if (!editor) return 0
  const doc = editor.state.doc
  const block = editor.lineBlockAt(doc.line(Math.min(doc.lines, Math.max(1, Math.floor(line) + 1))).from)
  const [first, last] = blockLines(editor, block)
  return contentTop(editor) + block.top + block.height * Math.min(1, Math.max(0, (line - first) / (last - first)))
}

function insertSnippet(snippet: string) {
  if (!editor || props.disabled) return
  editor.dispatch(editor.state.replaceSelection(snippet), { scrollIntoView: true, userEvent: 'input' })
  editor.focus()
}
defineExpose({
  insertSnippet, showImages, uploading, toggleImages: () => { showImages.value = !showImages.value }, requestMeasure: () => editor?.requestMeasure(), focus: () => editor?.focus(),
  scroller: () => editor?.scrollDOM, lineCount: () => editor?.state.doc.lines ?? 0, lineAtHeight, heightAtLine,
})
</script>

<template>
  <div class="markdown-source-wrapper">
    <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/webp" multiple hidden @change="chooseImages">
    <p v-if="uploading" class="image-message" role="status">画像を縮小して保存中…</p>
    <p v-if="imageMessage" class="image-message" role="alert">{{ imageMessage }}</p>
    <ContentImageLibrary v-if="showImages" :source="source" :disabled="disabled || uploading" @upload="fileInput?.click()" @insert="insertSnippet" @close="showImages = false; editor?.focus()" />
    <div ref="container" class="markdown-source-editor" />
  </div>
</template>

<style scoped>
.markdown-source-wrapper { display: flex; flex-direction: column; flex: 1; min-height: 0; overflow: hidden; }
.image-message { padding: 0 16px; font-size: .8rem; color: var(--color-error, #b42318); }
.image-message[role="status"] { color: var(--color-muted); }
.markdown-source-editor { position: relative; flex: 1; min-height: 0; overflow: clip; }
.markdown-source-editor:focus-within::after { content: ''; position: absolute; inset: 0; z-index: 10; border: 2px solid var(--color-accent); pointer-events: none; }
</style>
