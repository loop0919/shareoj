<script setup lang="ts">
import type { FeaturedPlan } from '~~/shared/types/featured'
import { featuredDay } from '~/utils/problem-choices'
const props = defineProps<{ plan: FeaturedPlan, rounds?: number }>()
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
            <template v-if="slot.kind === 'new'">
              <span class="schedule-entry"><DifficultyBadge :level="slot.difficulty" /><strong v-if="slot.trial">この問題</strong><UserLink v-else :handle="slot.writer" /></span>
              <span v-if="slot.title" class="schedule-title">{{ slot.title }}</span>
            </template>
            <span v-else class="schedule-open">{{ slot.kind === 'revival' ? '復刻' : '募集中' }}</span>
          </div>
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
/* Hallmark · component: featured schedule table · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: confirmed round · own problem · trial placement (dashed) · revival · open slot · link hover/focus via UserLink
 * Desktop: one row per round, Easy and Hard sharing the width equally. Under 40rem each round stacks with mono Easy/Hard labels, so nothing scrolls sideways. */
.featured-schedule { width: 100%; table-layout: fixed; border-collapse: collapse; font-size: .875rem; font-variant-numeric: tabular-nums; }
th, td { padding: 10px 12px; border-bottom: 1px solid var(--color-line); text-align: left; vertical-align: middle; }
thead th { padding-block: 8px; color: var(--color-muted); font-size: .75rem; font-weight: 500; }
thead th:first-child { width: 8.5rem; }
thead th + th { font-family: var(--font-code); }
tbody th { font-weight: 500; white-space: nowrap; }
.schedule-confirmed { margin-left: 8px; padding: 0 6px; border: 1px solid var(--color-accent); border-radius: 3px; color: var(--color-accent); font-size: .6875rem; font-weight: 600; }
.schedule-entry { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 2px 12px; min-width: 0; overflow-wrap: anywhere; }
.schedule-entry strong { color: var(--color-accent); font-weight: 650; }
.schedule-title { display: block; margin-top: 2px; color: var(--color-muted); font-size: .75rem; overflow-wrap: anywhere; }
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
