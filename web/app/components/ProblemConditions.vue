<script setup lang="ts">
import { conditionReset, problemContestFilters, problemSolvedFilters, problemSorts, type ConditionChip, type ConditionKey, type ProblemConditions, type ProblemContest, type ProblemSolved, type ProblemSort } from '~/utils/problem-conditions'

// The sort and the filters of the problem list, folded into one panel under a button. While it is closed, the conditions
// in force line up beside the button as chips, each with a way to take it off, so a list cut short never reads as
// problems gone missing. Each change goes up at once; there is no apply button.
// The page works out the chips (conditionChips) and what a new tag keeps, since it shows them too. The chips list solved
// only when the list really applied it, which it does not for a signed-out viewer, and the panel follows them.
// The default slot ends the button's row, for the page's own controls.
const props = defineProps<{ conditions: ProblemConditions, chips: ConditionChip[], tagKeep: Record<string, string>, signedIn: boolean, showTags: boolean }>()
const emit = defineEmits<{ change: [patch: Partial<ProblemConditions>], clear: [] }>()
const open = defineModel<boolean>('open', { required: true })
const route = useRoute()
const id = useId()
const levels = Array.from({ length: 10 }, (_, index) => index + 1)
// The fields show the conditions and send each pick up; the options are the only values they can send.
const sort = computed({ get: () => props.conditions.sort, set: (value: string) => emit('change', { sort: value as ProblemSort }) })
const solved = computed({ get: () => props.chips.some(chip => chip.key === 'solved') ? props.conditions.solved : '', set: (value: string) => emit('change', { solved: value as ProblemSolved }) })
const contest = computed({ get: () => props.conditions.contest, set: (value: string) => emit('change', { contest: value as ProblemContest }) })
function pickLevel(bound: 'min' | 'max', event: Event) {
  const level = Number((event.target as HTMLSelectElement).value)
  emit('change', bound === 'min' ? { min: level } : { max: level })
}
// Only the condition taken off goes up: a second chip taken off before the list catches up then keeps the first one off.
const remove = (key: ConditionKey) => emit('change', conditionReset(key))
</script>
<template>
  <div class="problem-conditions">
    <div class="conditions-bar">
      <button type="button" class="editor-button conditions-toggle" :aria-expanded="open" :aria-controls="`${id}-panel`" @click="open = !open">
        絞り込み・並び替え<span v-if="chips.length" class="conditions-count"><span class="visually-hidden">（効いている条件</span>{{ chips.length }}<span class="visually-hidden">件）</span></span>
        <svg class="editor-icon conditions-chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
      </button>
      <ul v-if="!open && chips.length" class="conditions-chips" aria-label="効いている条件">
        <li v-for="chip in chips" :key="chip.key">{{ chip.label }}<button type="button" :aria-label="`${chip.label}を外す`" :title="`${chip.label}を外す`" @click="remove(chip.key)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg></button></li>
      </ul>
      <slot />
    </div>
    <div v-if="open" :id="`${id}-panel`" class="conditions-panel">
      <div class="conditions-fields">
        <FilterSelect v-model="sort" label="並び順" :options="problemSorts" />
        <div class="conditions-field">
          <span class="conditions-label" aria-hidden="true">解答状況</span>
          <StatusFilter v-model="solved" label="解答状況" :options="problemSolvedFilters" :disabled="!signedIn" />
          <p v-if="!signedIn" class="conditions-note"><NuxtLink :to="{ path: '/login', query: { next: route.fullPath } }">ログイン</NuxtLink>すると使えます</p>
        </div>
        <FilterSelect v-model="contest" label="コンテスト" :options="problemContestFilters" />
        <div class="conditions-field" role="group" :aria-labelledby="`${id}-difficulty`">
          <span :id="`${id}-difficulty`" class="conditions-label">難易度</span>
          <span class="conditions-range">
            <select aria-label="難易度の下限" :value="conditions.min" @change="pickLevel('min', $event)"><option v-for="level in levels" :key="level" :value="level">Lv.{{ level }}</option></select>
            <span aria-hidden="true">〜</span>
            <select aria-label="難易度の上限" :value="conditions.max" @change="pickLevel('max', $event)"><option v-for="level in levels" :key="level" :value="level">Lv.{{ level }}</option></select>
          </span>
        </div>
        <!-- The same rule as before the panel: a viewer who hides problem tags gets no tag search, unless a link opened a tagged list. -->
        <div v-if="showTags || conditions.tag" class="conditions-field">
          <span class="conditions-label" aria-hidden="true">タグ</span>
          <TagFilter kind="problem" :tag="conditions.tag" to="/problems" :keep="tagKeep" />
        </div>
      </div>
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
/* Each field puts its name above its control, the selects' own labels included. */
.conditions-field, .conditions-fields > .filter-select { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; min-width: 0; }
.conditions-label { color: var(--color-muted); font-size: .8125rem; white-space: nowrap; }
.conditions-field > .status-filter, .conditions-field > .tag-filter { margin-bottom: 0; }
.conditions-note { margin: 0; color: var(--color-muted); font-size: .8125rem; }
.conditions-range { display: flex; align-items: center; gap: 8px; }
.conditions-range select { min-height: 36px; padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; font-size: .875rem; font-variant-numeric: tabular-nums; cursor: pointer; }
.conditions-range select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.conditions-foot { display: flex; justify-content: flex-end; margin-top: 16px; }
.conditions-foot .editor-button { min-height: 36px; padding-inline: 12px; font-size: .875rem; }
@media (hover: hover) { .conditions-range select:hover { border-color: var(--color-muted); } }
@media (prefers-reduced-motion: reduce) { .conditions-chevron { transition: none; } }
/* One field a row; 1rem keeps a phone from zooming into a list when it opens. */
@media (max-width: 39.999rem) {
  .conditions-panel { padding: 16px; }
  .conditions-fields { grid-template-columns: 1fr; }
  .conditions-range select { min-height: 44px; font-size: 1rem; }
}
</style>
