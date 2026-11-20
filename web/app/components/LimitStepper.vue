<script setup lang="ts">
const props = defineProps<{ id: string, modelValue: string, options: number[], defaultValue: number, step?: number, label: string, labelledby: string, disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const text = ref(props.modelValue)
const focused = ref(false)
const minimum = computed(() => props.options[0]!)
const maximum = computed(() => props.options[props.options.length - 1]!)
watch(() => props.modelValue, value => { if (!focused.value) text.value = value })
function normalize(raw: string) {
  const value = raw.trim() ? Number(raw) : props.defaultValue
  const step = props.step ?? 1
  return Math.max(minimum.value, Math.min(maximum.value, Math.round((Number.isFinite(value) ? value : props.defaultValue) / step) * step))
}
function update(event: Event) {
  text.value = (event.target as HTMLInputElement).value
  // Keep saves and previews valid while allowing intermediate text during typing.
  emit('update:modelValue', String(normalize(text.value)))
}
function commit() {
  text.value = String(normalize(text.value))
  emit('update:modelValue', text.value)
}
function blur() {
  commit()
  focused.value = false
}
function move(direction: number) {
  if (props.disabled) return
  const current = normalize(text.value)
  const value = direction > 0
    ? props.options.find(value => value > current) ?? maximum.value
    : [...props.options].reverse().find(value => value < current) ?? minimum.value
  text.value = String(value)
  emit('update:modelValue', text.value)
}
function onKey(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    event.preventDefault()
    move(event.key === 'ArrowUp' ? 1 : -1)
  } else if (event.key === 'Enter' || ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's')) {
    commit()
  }
}
</script>

<template>
  <div class="limit-stepper" :aria-disabled="disabled">
    <input :id="id" class="limit-stepper-value" type="text" inputmode="numeric" role="spinbutton" :value="text" :placeholder="String(defaultValue)" :disabled="disabled" :aria-labelledby="labelledby" :aria-valuenow="Number(modelValue)" :aria-valuemin="minimum" :aria-valuemax="maximum" @focus="focused = true" @input="update" @blur="blur" @keydown="onKey">
    <div class="limit-stepper-buttons">
      <button type="button" :aria-label="`${label}を増やす`" :disabled="disabled || Number(modelValue) >= maximum" @click="move(1)"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 10 4-4 4 4" /></svg></button>
      <button type="button" :aria-label="`${label}を減らす`" :disabled="disabled || Number(modelValue) <= minimum" @click="move(-1)"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg></button>
    </div>
  </div>
</template>

<style scoped>
.limit-stepper { display: flex; align-items: stretch; justify-content: space-between; min-height: 40px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font-size: .875rem; }
.limit-stepper .limit-stepper-value { width: 100%; min-width: 0; align-self: stretch; padding: 6px 8px; border: 0; border-radius: 4px 0 0 4px; font-variant-numeric: tabular-nums; }
.limit-stepper-buttons { display: flex; flex-direction: column; width: 32px; flex-shrink: 0; border-left: 1px solid var(--color-line); }
.limit-stepper button { display: grid; place-items: center; flex: 1; min-height: 20px; padding: 0; border: 0; background: transparent; color: var(--color-muted); cursor: pointer; }
.limit-stepper button + button { border-top: 1px solid var(--color-line); }
.limit-stepper svg { width: 14px; height: 14px; fill: none; stroke: currentColor; stroke-width: 1.5; }
.limit-stepper .limit-stepper-value:focus-visible, .limit-stepper button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.limit-stepper button:active:not(:disabled) { background: var(--color-accent-soft); color: var(--color-accent); }
.limit-stepper[aria-disabled="true"] { opacity: .55; }
@media (hover: hover) { .limit-stepper button:hover:not(:disabled) { background: var(--color-accent-soft); color: var(--color-accent); } }
@media (pointer: coarse) { .limit-stepper-buttons { width: 44px; } .limit-stepper button { min-height: 32px; } }
</style>
