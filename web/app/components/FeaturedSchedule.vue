<script setup lang="ts">
import type { FeaturedPlan } from '~~/shared/types/featured'
import { featuredDay } from '~/utils/problem-choices'
// trialLabel says what the chosen problem's placement means: before applying it is a what-if.
const props = defineProps<{ plan: FeaturedPlan, rounds?: number, trialLabel?: string }>()
const shown = computed(() => props.plan.rounds.slice(0, props.rounds ?? props.plan.rounds.length))
const slotNames = { easy: 'Easy', hard: 'Hard' } as const
</script>

<template>
  <table class="featured-schedule">
    <thead><tr><th scope="col">出題日</th><th scope="col">Easy</th><th scope="col">Hard</th></tr></thead>
    <tbody>
      <tr v-for="round in shown" :key="round.scheduledAt">
        <th scope="row"><time :datetime="round.scheduledAt">{{ featuredDay(round.scheduledAt) }}</time><span v-if="round.confirmed" class="schedule-confirmed">確定</span></th>
        <td v-for="slot in round.slots" :key="slot.slot" :data-slot="slotNames[slot.slot]" :data-mine="slot.mine && !slot.trial || undefined" :data-trial="slot.trial || undefined">
          <div class="schedule-cell">
            <template v-if="slot.kind === 'new' && slot.title">
              <span class="schedule-entry schedule-own"><DifficultyBadge :level="slot.difficulty" /><strong>{{ slot.title }}</strong></span>
              <span class="schedule-sub">{{ slot.trial ? trialLabel ?? '応募した場合' : '応募中' }}</span>
            </template>
            <!-- Others' problems show only the level: the schedule never says who wrote them. -->
            <span v-else-if="slot.kind === 'new'" class="schedule-entry"><DifficultyBadge :level="slot.difficulty" /></span>
            <span v-else class="schedule-open">{{ slot.kind === 'revival' ? '復刻' : '募集中' }}</span>
          </div>
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
/* Hallmark · component: featured schedule table · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: confirmed round · own problem (title) · chosen problem (title, dashed) · others' problem (level only) · revival · open slot
 * Desktop: one row per round, Easy and Hard sharing the width equally. Under 40rem each round stacks with mono Easy/Hard labels, so nothing scrolls sideways. */
.featured-schedule { width: 100%; table-layout: fixed; border-collapse: collapse; font-size: .875rem; font-variant-numeric: tabular-nums; }
th, td { padding: 10px 12px; border-bottom: 1px solid var(--color-line); text-align: left; vertical-align: middle; }
thead th { padding-block: 8px; color: var(--color-muted); font-size: .75rem; font-weight: 500; }
thead th:first-child { width: 8.5rem; }
thead th + th { font-family: var(--font-code); }
tbody th { font-weight: 500; white-space: nowrap; }
.schedule-confirmed { margin-left: 8px; padding: 0 6px; border: 1px solid var(--color-accent); border-radius: 3px; color: var(--color-accent); font-size: .6875rem; font-weight: 600; }
.schedule-entry { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 2px 12px; min-width: 0; overflow-wrap: anywhere; }
.schedule-own strong { font-size: .9375rem; font-weight: 650; line-height: 1.6; }
td[data-trial] .schedule-own strong { color: var(--color-accent); }
.schedule-sub { display: block; margin-top: 2px; color: var(--color-muted); font-size: .75rem; }
.schedule-open { color: var(--color-muted); }
td[data-mine] { background: var(--color-accent-soft); }
td[data-trial] { background: var(--color-accent-soft); outline: 1px dashed var(--color-accent); outline-offset: -5px; }
@media (max-width: 40rem) {
  thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
  tr { display: block; padding-block: 10px; border-bottom: 1px solid var(--color-line); }
  tbody th, td { display: block; padding: 2px 0; border: 0; }
  tbody th { margin-bottom: 4px; }
  td { display: grid; grid-template-columns: 3.25rem minmax(0, 1fr); align-items: baseline; }
  td::before { content: attr(data-slot); color: var(--color-muted); font-family: var(--font-code); font-size: .75rem; }
  td[data-mine], td[data-trial] { margin: 2px -8px 0; padding: 6px 8px; border-radius: 4px; }
  td[data-trial] { outline-offset: -1px; }
}
</style>
