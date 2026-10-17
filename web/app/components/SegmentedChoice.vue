<script setup lang="ts" generic="T extends string">
defineProps<{
  legend: string
  name: string
  options: { value: T, label: string, disabled?: boolean, describedby?: string }[]
  // The options can speak for themselves; the legend still names the group for screen readers.
  hideLegend?: boolean
  disabled?: boolean
}>()
const selected = defineModel<T>({ required: true })
</script>

<template>
  <fieldset class="segments" :disabled="disabled">
    <legend :data-hidden="hideLegend || undefined">{{ legend }}</legend>
    <div>
      <label v-for="option in options" :key="option.value">
        <input v-model="selected" type="radio" :name="name" :value="option.value" :disabled="option.disabled" :aria-describedby="option.describedby">
        <slot name="icon" :value="option.value" />{{ option.label }}
      </label>
    </div>
  </fieldset>
</template>

<style scoped>
/* Hallmark · component: two-way segmented radio · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: default · hover · focus-visible · active · checked (border, fill, weight) · option disabled · group disabled
 * Equal halves always fit one row; the whole half is the radio's hit area. */
.segments { min-width: 0; margin: 0; padding: 0; border: 0; }
legend { margin-bottom: 8px; padding: 0; font-size: .875rem; font-weight: 600; line-height: 32px; }
legend[data-hidden] { position: absolute; width: 1px; height: 1px; margin: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
div { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
label { position: relative; display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 44px; padding: 6px 10px; border: 1px solid var(--color-line); border-radius: 999px; font-size: .875rem; white-space: nowrap; cursor: pointer; }
input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
label:has(input:checked) { border-color: var(--color-accent); background: var(--color-accent-soft); font-weight: 600; }
label:has(input:checked) :slotted(svg) { color: var(--color-accent); }
label:has(input:focus-visible) { outline: 3px solid var(--color-accent); outline-offset: 2px; }
label:active { transform: translateY(1px); }
label:has(input:disabled) { opacity: .55; cursor: not-allowed; }
input:disabled { cursor: not-allowed; }
:slotted(svg) { flex: none; color: var(--color-muted); }
@media (hover: hover) {
  label:not(:has(input:checked)):not(:has(input:disabled)):hover { background: var(--color-surface); }
}
@media (max-width: 400px) {
  label { padding-inline: 6px; gap: 4px; font-size: .8125rem; }
}
</style>
