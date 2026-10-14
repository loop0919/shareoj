<script setup lang="ts">
import { basicSetup } from 'codemirror'
import { codeLanguage, codeLanguageSupport } from '~/utils/code-language'
import { Compartment, EditorState } from '@codemirror/state'
import { indentWithTab } from '@codemirror/commands'
import { indentUnit, defaultHighlightStyle, HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorView, keymap } from '@codemirror/view'

const source = defineModel<string>({ required: true })
const props = withDefaults(defineProps<{ disabled?: boolean, readonly?: boolean, label?: string, runtime?: string, limit?: number }>(), { limit: 65536 })
const labelId = useId()
const container = ref<HTMLDivElement>()
const bytes = computed(() => new TextEncoder().encode(source.value).length)
const language = computed(() => codeLanguage(props.runtime ?? ''))
const { settings, indentation } = useEditorSettings('code', language)
const languageConfig = new Compartment()
const indentConfig = new Compartment()
const indentExtensions = () => [indentUnit.of(indentation.value), EditorState.tabSize.of(settings.value.width)]
const editable = new Compartment()
let editor: EditorView | undefined
const themeExtension = useCodeMirrorTheme(() => editor)
const editing = () => [EditorState.readOnly.of(Boolean(props.disabled || props.readonly)), EditorView.editable.of(!props.disabled && !props.readonly)]

onMounted(() => {
  editor = new EditorView({
    parent: container.value,
    doc: source.value,
    extensions: [
      basicSetup,
      themeExtension(),
      languageConfig.of(codeLanguageSupport(language.value)),
      syntaxHighlighting(HighlightStyle.define(defaultHighlightStyle.specs.map(style => ({
        ...style,
        ...(style.color ? { color: `light-dark(${style.color}, color-mix(in srgb, ${style.color} 35%, white))` } : {}),
      })))),
      keymap.of([indentWithTab]),
      indentConfig.of(indentExtensions()),
      editable.of(editing()),
      EditorView.contentAttributes.of({ 'aria-labelledby': labelId, 'aria-multiline': 'true', ...(props.readonly ? { tabindex: '0', 'aria-readonly': 'true' } : {}) }),
      EditorView.updateListener.of(update => {
        if (update.docChanged) source.value = update.state.doc.toString()
      }),
      EditorView.theme({
        '&': { height: '100%', color: 'var(--color-ink)', backgroundColor: 'var(--color-paper)' },
        '&.cm-focused': { outline: 'none' },
        '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-code)', fontSize: '14px', lineHeight: '22px' },
        '.cm-content': { padding: '0' },
        '.cm-line': { padding: '0 16px' },
        '.cm-gutters': { backgroundColor: 'var(--color-surface)', color: 'var(--color-muted)', borderRight: '1px solid var(--color-line)' },
        '.cm-lineNumbers .cm-gutterElement': { minWidth: '44px', paddingRight: '10px' },
        '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: props.readonly ? 'transparent' : 'var(--color-accent-soft)' },
      }),
    ],
  })
})
watch(() => [props.disabled, props.readonly], () => editor?.dispatch({ effects: editable.reconfigure(editing()) }))
watch(() => [settings.value.style, settings.value.width], () => editor?.dispatch({ effects: indentConfig.reconfigure(indentExtensions()) }))
watch(language, value => editor?.dispatch({ effects: languageConfig.reconfigure(codeLanguageSupport(value)) }))
watch(source, value => {
  if (editor && value !== editor.state.doc.toString()) {
    editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value } })
  }
})
onBeforeUnmount(() => editor?.destroy())
</script>

<template>
  <div class="source-code-editor" :class="{ 'is-disabled': disabled, 'is-readonly': readonly }">
    <div class="pane-heading">
      <span :id="labelId">{{ label || 'ソースコード' }}</span>
      <div class="source-editor-actions"><span class="byte-count">{{ bytes.toLocaleString('en-US') }}{{ readonly ? ' bytes' : ` / ${limit.toLocaleString('en-US')} bytes` }}</span><EditorSettings kind="code" :language="language" v-if="!readonly" :disabled="disabled" /></div>
    </div>
    <div ref="container" class="code-surface" />
  </div>
</template>

<style scoped>
.source-code-editor { min-width: 0; margin-top: 20px; border: 1px solid var(--color-line); border-radius: 4px; overflow: hidden; }
.pane-heading { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--color-line); font-size: .875rem; }
.source-editor-actions { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.byte-count { font-size: .75rem; color: var(--color-muted); }
.code-surface { position: relative; height: 384px; min-height: 180px; overflow: hidden; resize: vertical; }
.code-surface:focus-within::after { content: ""; position: absolute; inset: 0; z-index: 10; border: 2px solid var(--color-accent); pointer-events: none; }
.is-readonly .code-surface { height: auto; min-height: 0; resize: none; }
.is-readonly :deep(.cm-scroller) { max-height: 320px; }
.is-disabled { opacity: .6; }
</style>
