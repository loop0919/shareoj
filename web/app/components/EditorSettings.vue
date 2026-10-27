<script setup lang="ts">
import { vDialogDismiss } from '~/utils/dialog-dismiss'
const props = defineProps<{ kind: 'code' | 'markdown', disabled?: boolean, language?: string }>()
const { settings, saveSettings } = useEditorSettings(props.kind, () => props.language ?? 'cpp')
const dialog = ref<HTMLDialogElement>()
const titleId = useId()
</script>

<template>
  <button class="editor-button settings-button" type="button" aria-label="エディタ設定" title="エディタ設定" :disabled="disabled" @click="dialog?.showModal()">
    <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 3 .6-2h4.8l.6 2 2 1.2 2.1-.5 2.4 4.2-1.5 1.5v2.3l1.5 1.5-2.4 4.2-2.1-.5-2 1.2-.6 2H9l-.6-2-2-1.2-2.1.5-2.4-4.2 1.5-1.5V9.4L1.9 7.9l2.4-4.2 2.1.5Z" transform="translate(0 1)" /><circle cx="12" cy="12" r="3" /></svg>
  </button>
  <Teleport to="body">
    <dialog v-dialog-dismiss ref="dialog" class="editor-settings-dialog" :aria-labelledby="titleId">
      <header class="settings-heading">
        <h2 :id="titleId">エディタ設定（{{ kind === 'markdown' ? 'Markdown' : 'Code' }}）</h2>
        <button class="editor-button settings-close" type="button" aria-label="エディタ設定を閉じる" @click="dialog?.close()"><span aria-hidden="true">×</span></button>
      </header>
      <label>インデント方式
        <select v-model="settings.style" @change="saveSettings"><option value="space">space（スペース）</option><option value="tab">tab（タブ）</option></select>
      </label>
      <label>インデント幅
        <select v-model="settings.width" @change="saveSettings"><option v-for="width in 8" :key="width" :value="width">{{ width }}</option></select>
      </label>
      <label v-if="kind === 'markdown'" class="switch-setting">編集欄とプレビューのスクロールを連動
        <span class="switch-control">
          <span class="switch-state" aria-hidden="true">{{ settings.scrollSync ? 'ON' : 'OFF' }}</span>
          <button type="button" role="switch" class="switch" :aria-checked="settings.scrollSync" @click="settings.scrollSync = !settings.scrollSync; saveSettings()" />
        </span>
      </label>
    </dialog>
  </Teleport>
</template>

<style scoped>
.settings-button { margin-left: auto; display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; padding: 5px; }
.settings-button svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.6; stroke-linecap: round; stroke-linejoin: round; }
.editor-settings-dialog { width: min(460px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: 32px; border: 1px solid var(--color-line); border-radius: 8px; color: var(--color-ink); background: var(--color-paper); word-break: auto-phrase; }
.editor-settings-dialog::backdrop { background: var(--color-dialog-backdrop); }
.settings-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 20px; }
.settings-close { display: inline-flex; align-items: center; justify-content: center; width: 36px; height: 36px; padding: 0; font-size: 1.5rem; }
h2 { margin: 0; font-size: 1.125rem; }
label { display: grid; gap: 8px; margin-block: 16px; font-size: .875rem; }
label:last-child { margin-bottom: 0; }
select { width: 100%; padding: 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-ink); background: var(--color-paper); font: inherit; }
select:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.switch-setting { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.switch-control { display: inline-flex; align-items: center; gap: 8px; flex-shrink: 0; }
.switch-state { min-width: 3ch; font-size: .75rem; color: var(--color-muted); text-align: end; }
.switch { position: relative; width: 44px; height: 24px; padding: 0; border: 1px solid var(--color-line); border-radius: 12px; background: var(--color-surface); cursor: pointer; }
.switch::after { content: ''; position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--color-muted); transition: transform .15s; }
.switch[aria-checked="true"] { border-color: var(--color-accent); background: var(--color-accent); }
.switch[aria-checked="true"]::after { transform: translateX(20px); background: var(--color-paper); }
.switch:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) { .switch::after { transition: none; } }
@media (max-width: 30rem) { .editor-settings-dialog { padding: 24px; } }
</style>
