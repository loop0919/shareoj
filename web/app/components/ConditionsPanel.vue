<script setup lang="ts">
// The shell of a list's sort and filters, folded into one panel under a button. While it is closed, the conditions in
// force line up beside the button as chips, each with a way to take it off, so a list cut short never reads as rows gone
// missing. The list works out the chips; the default slot holds its fields, and the bar slot ends the button's row, for
// the page's own controls.
// focus moves the focus to the button, for a control that leaves the page when it takes effect.
defineProps<{ chips: readonly { key: string, label: string }[] }>()
const emit = defineEmits<{ remove: [key: string], clear: [] }>()
const open = defineModel<boolean>('open', { required: true })
const id = useId()
const toggle = ref<HTMLButtonElement>()
const focus = () => toggle.value?.focus()
defineExpose({ focus })
// The chip goes with the condition, so the focus goes to the button.
function remove(key: string) {
  emit('remove', key)
  focus()
}
</script>
<template>
  <div class="conditions">
    <div class="conditions-bar">
      <button ref="toggle" type="button" class="editor-button conditions-toggle" :aria-expanded="open" :aria-controls="open ? `${id}-panel` : undefined" @click="open = !open">
        絞り込み・並び替え<span v-if="chips.length" class="conditions-count"><span class="visually-hidden">（効いている条件</span>{{ chips.length }}<span class="visually-hidden">件）</span></span>
        <svg class="editor-icon conditions-chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
      </button>
      <ul v-if="!open && chips.length" class="conditions-chips" aria-label="効いている条件">
        <li v-for="chip in chips" :key="chip.key">{{ chip.label }}<button type="button" :aria-label="`${chip.label}を外す`" :title="`${chip.label}を外す`" @click="remove(chip.key)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg></button></li>
      </ul>
      <slot name="bar" />
    </div>
    <div v-if="open" :id="`${id}-panel`" class="conditions-panel">
      <div class="conditions-fields"><slot /></div>
      <div class="conditions-foot"><button type="button" class="editor-button" @click="emit('clear')">条件をすべて外す</button></div>
    </div>
  </div>
</template>
<style scoped>
.conditions-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
.conditions-toggle { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 4px 12px; font-size: .875rem; }
.conditions-toggle[aria-expanded="true"] { border-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-accent); }
.conditions-count { min-width: 1.5em; padding: 0 6px; border-radius: 999px; background: var(--color-accent); color: var(--color-paper); font-size: .75rem; font-weight: 700; line-height: 1.6; text-align: center; font-variant-numeric: tabular-nums; }
.conditions-chevron { width: 16px; height: 16px; transition: transform .15s ease; }
.conditions-toggle[aria-expanded="true"] .conditions-chevron { transform: rotate(180deg); }
.conditions-chips { display: flex; flex-wrap: wrap; gap: 6px; min-width: 0; margin: 0; padding: 0; list-style: none; }
.conditions-chips li { display: inline-flex; align-items: center; gap: 2px; min-width: 0; padding: 1px 2px 1px 10px; border: 1px solid var(--color-line); border-radius: 999px; background: var(--color-surface); font-size: .8125rem; line-height: 1.6; overflow-wrap: anywhere; }
.conditions-chips button { display: inline-grid; flex: none; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 50%; background: transparent; color: var(--color-muted); cursor: pointer; }
.conditions-chips button .editor-icon { width: 14px; height: 14px; }
.conditions-chips button:hover { background: var(--color-line); color: var(--color-ink); }
.conditions-chips button:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 0; }
.conditions-panel { margin-top: 12px; padding: 16px 20px; border: 1px solid var(--color-line); border-radius: 6px; background: var(--color-surface); }
.conditions-fields { display: grid; grid-template-columns: repeat(auto-fill, minmax(17.5rem, 1fr)); gap: 16px 24px; }
/* Each field puts its name above its control, the selects' own labels included. A field the list builds of its own
   parts is a .conditions-field, its name a .conditions-label. */
.conditions-fields > :slotted(.conditions-field), .conditions-fields > :slotted(.filter-select) { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; min-width: 0; }
:slotted(.conditions-label) { color: var(--color-muted); font-size: .8125rem; white-space: nowrap; }
.conditions-foot { display: flex; justify-content: flex-end; margin-top: 16px; }
.conditions-foot .editor-button { min-height: 36px; padding-inline: 12px; font-size: .875rem; }
@media (prefers-reduced-motion: reduce) { .conditions-chevron { transition: none; } }
/* One field a row. */
@media (max-width: 39.999rem) {
  .conditions-panel { padding: 16px; }
  .conditions-fields { grid-template-columns: 1fr; }
}
</style>
