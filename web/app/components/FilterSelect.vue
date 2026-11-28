<script setup lang="ts">
// One filter as a labelled list to pick from, for a page with more than one filter: rows of buttons take the page's
// width twice over. '' shows everything, and each choice says how many rows it would show.
// Without counts it is a plain choice among the options, such as an order: nothing is hidden, so there is no すべて
// and no mark that rows are held back.
// groups follow the options under headings of their own, for choices that fall into kinds.
type Option = { key: string, label: string }
const props = defineProps<{ label: string, options: readonly Option[], groups?: readonly { label: string, options: readonly Option[] }[], counts?: Record<string, number> }>()
const selected = defineModel<string>({ required: true })
const id = useId()
const total = computed(() => Object.values(props.counts ?? {}).reduce((sum, count) => sum + count, 0))
</script>
<template>
  <div class="filter-select">
    <label :for="id">{{ label }}</label>
    <select :id="id" v-model="selected" :class="{ 'filter-select-on': counts && selected }">
      <option v-if="counts" value="">すべて（{{ total }}）</option>
      <option v-for="option in options" :key="option.key" :value="option.key">{{ option.label }}<template v-if="counts">（{{ counts[option.key] ?? 0 }}）</template></option>
      <optgroup v-for="group in groups" :key="group.label" :label="group.label">
        <option v-for="option in group.options" :key="option.key" :value="option.key">{{ option.label }}<template v-if="counts">（{{ counts[option.key] ?? 0 }}）</template></option>
      </optgroup>
    </select>
  </div>
</template>
<style scoped>
.filter-select { display: flex; align-items: center; gap: 8px; min-width: 0; }
.filter-select label { color: var(--color-muted); font-size: .8125rem; white-space: nowrap; }
/* A long option, such as a problem's title, must not make the select wider than the place it is given. */
.filter-select select { min-width: 0; max-width: 100%; min-height: 36px; padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; font-size: .875rem; font-variant-numeric: tabular-nums; cursor: pointer; }
.filter-select select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
/* A filter that hides rows stands out, since the closed list shows no other sign of it. */
.filter-select select.filter-select-on { border-color: var(--color-accent); background-color: var(--color-accent-soft); font-weight: 600; }
.filter-select option { background-color: var(--color-paper); font-weight: 400; }
.filter-select optgroup { background-color: var(--color-paper); }
@media (hover: hover) { .filter-select select:hover { border-color: var(--color-muted); } .filter-select select.filter-select-on:hover { border-color: var(--color-accent); } }
/* 1rem keeps a phone from zooming into the list when it opens. */
@media (max-width: 39.999rem) { .filter-select select { min-height: 44px; font-size: 1rem; } }
</style>
