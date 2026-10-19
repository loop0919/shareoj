<script setup lang="ts">
// A small on/off switch with its label beside it, for lists and toolbars. The parent places it through a class.
// label names the switch for screen readers when the visible text is short, as in a table row. disabled is for a switch
// outside a disabled fieldset; attributes on the component land on the label, not the input.
defineProps<{ describedby?: string, label?: string, disabled?: boolean }>()
const on = defineModel<boolean>({ required: true })
</script>

<template>
  <label class="inline-switch">
    <input v-model="on" type="checkbox" role="switch" :aria-describedby="describedby" :aria-label="label" :disabled="disabled">
    <span class="switch-track" aria-hidden="true"><span class="switch-thumb" /></span>
    <slot />
  </label>
</template>

<style scoped>
/* Hallmark · component: inline switch · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: off · on · hover · focus-visible · active · disabled · reduced motion
 * A switch is a checkbox: the input covers the whole label, so a click toggles it without moving focus or the page. */
.inline-switch { position: relative; display: inline-flex; align-items: center; gap: 8px; min-height: 44px; font-size: .8125rem; white-space: nowrap; cursor: pointer; }
.inline-switch input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.switch-track { position: relative; flex: none; width: 34px; height: 20px; border: 1px solid var(--color-muted); border-radius: 999px; background: var(--color-surface); }
.switch-thumb { position: absolute; top: 2px; left: 2px; width: 14px; height: 14px; border-radius: 50%; background: var(--color-muted); transition: transform 120ms cubic-bezier(.2, 0, 0, 1); }
.inline-switch input:checked + .switch-track { border-color: var(--color-accent); background: var(--color-accent); }
.inline-switch input:checked + .switch-track .switch-thumb { background: var(--color-paper); transform: translateX(14px); }
.inline-switch input:focus-visible + .switch-track { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.inline-switch input:active + .switch-track .switch-thumb { width: 16px; }
.inline-switch:has(input:disabled) { opacity: .55; cursor: not-allowed; }
.inline-switch input:disabled { cursor: not-allowed; }
@media (hover: hover) {
  .inline-switch:hover .switch-track { border-color: var(--color-ink); }
  .inline-switch:hover input:checked + .switch-track { border-color: var(--color-accent); }
}
@media (prefers-reduced-motion: reduce) {
  .switch-thumb { transition: none; }
}
</style>
