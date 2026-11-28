<script setup lang="ts">
import { conditionReset, problemContestFilters, problemSolvedFilters, problemSorts, type ConditionChip, type ConditionKey, type ProblemConditions, type ProblemContest, type ProblemSolved, type ProblemSort } from '~/utils/problem-conditions'

// The sort and the filters of the problem list, in the panel that ConditionsPanel folds under a button. Each change goes
// up at once; there is no apply button.
// The page works out the chips (conditionChips) and what a new tag keeps, since it shows them too. The chips list solved
// only when the list really applied it, which it does not for a signed-out viewer, and the panel follows them.
// The default slot ends the button's row, for the page's own controls.
// focus moves the focus to the button, for a control of the page's that leaves the page when it takes effect.
const props = defineProps<{ conditions: ProblemConditions, chips: ConditionChip[], tagKeep: Record<string, string>, signedIn: boolean, showTags: boolean }>()
const emit = defineEmits<{ change: [patch: Partial<ProblemConditions>], clear: [] }>()
const open = defineModel<boolean>('open', { required: true })
const route = useRoute()
const id = useId()
const shell = ref<{ focus: () => void }>()
const focus = () => shell.value?.focus()
defineExpose({ focus })
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
function remove(key: string) {
  emit('change', conditionReset(key as ConditionKey))
}
</script>
<template>
  <ConditionsPanel ref="shell" v-model:open="open" :chips="chips" @remove="remove" @clear="emit('clear')">
    <template #bar><slot /></template>
    <FilterSelect v-model="sort" label="並び順" :options="problemSorts" />
    <div class="conditions-field">
      <span class="conditions-label" aria-hidden="true">解答状況</span>
      <StatusFilter v-model="solved" label="解答状況" :options="problemSolvedFilters" :disabled="!signedIn" :describedby="signedIn ? undefined : `${id}-solved-note`" />
      <p v-if="!signedIn" :id="`${id}-solved-note`" class="conditions-note"><NuxtLink :to="{ path: '/login', query: { next: route.fullPath } }">ログイン</NuxtLink>すると使えます</p>
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
  </ConditionsPanel>
</template>
<style scoped>
.conditions-field > .status-filter, .conditions-field > .tag-filter { margin-bottom: 0; }
.conditions-note { margin: 0; color: var(--color-muted); font-size: .8125rem; }
.conditions-range { display: flex; align-items: center; gap: 8px; }
.conditions-range select { min-height: 36px; padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; font-size: .875rem; font-variant-numeric: tabular-nums; cursor: pointer; }
.conditions-range select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
@media (hover: hover) { .conditions-range select:hover { border-color: var(--color-muted); } }
/* 1rem keeps a phone from zooming into a list when it opens. */
@media (max-width: 39.999rem) { .conditions-range select { min-height: 44px; font-size: 1rem; } }
</style>
