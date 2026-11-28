<script setup lang="ts">
// '' shows everything. Counts stay hidden until the whole list is known, since a partial count misleads.
// label names the group, for a page with a second filter that is not by status. disabled turns every button off, for a
// filter the viewer cannot use yet; describedby names what says why.
const props = withDefaults(defineProps<{ options: readonly { key: string, label: string }[], counts?: Record<string, number>, label?: string, disabled?: boolean, describedby?: string }>(), { label: '状態で絞り込む' })
const selected = defineModel<string>({ required: true })
const total = computed(() => props.counts && Object.values(props.counts).reduce((sum, count) => sum + count, 0))
</script>
<template>
  <div class="status-filter" role="group" :aria-label="label" :aria-describedby="describedby">
    <button type="button" :aria-pressed="!selected" :disabled="disabled" @click="selected = ''">すべて<span v-if="counts" class="status-count">{{ total }}</span></button>
    <button v-for="option in options" :key="option.key" type="button" :aria-pressed="selected === option.key" :disabled="disabled" @click="selected = option.key">{{ option.label }}<span v-if="counts" class="status-count">{{ counts[option.key] ?? 0 }}</span></button>
  </div>
</template>
<style scoped>
.status-filter { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 20px; }
.status-filter button { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 2px 14px; border: 1px solid var(--color-line); border-radius: 999px; background: var(--color-paper); color: var(--color-muted); font: inherit; font-size: .8125rem; white-space: nowrap; cursor: pointer; }
.status-filter button:hover:not(:disabled) { border-color: var(--color-accent); color: var(--color-ink); }
.status-filter button:disabled { opacity: .55; cursor: not-allowed; }
.status-filter button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.status-filter button[aria-pressed="true"] { border-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-ink); font-weight: 600; }
.status-count { color: var(--color-muted); font-weight: 400; font-variant-numeric: tabular-nums; }
</style>
