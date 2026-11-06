<script setup lang="ts">
import { contestCountdown, contestDate, type ScheduledContest } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'

defineProps<{ items: ScheduledContest[], now: number }>()
</script>

<template>
  <section class="upcoming-contests" aria-labelledby="upcoming-contests-title">
    <h2 id="upcoming-contests-title">もうすぐ開催のコンテスト</h2>
    <ul>
      <li v-for="contest in items" :key="contest.id">
        <time :datetime="contest.startsAt">{{ contestDate(contest.startsAt) }}</time>
        <NuxtLink :to="`/contests/${shortId(contest.id)}`">{{ contest.title }}</NuxtLink>
        <span>{{ contestCountdown(contest.startsAt, now) }}</span>
      </li>
    </ul>
    <NuxtLink class="upcoming-more" to="/contests">コンテスト一覧<span aria-hidden="true">→</span></NuxtLink>
  </section>
</template>

<style scoped>
.upcoming-contests { margin-block: 24px 48px; padding-top: 28px; border-top: 2px solid var(--color-accent); }
.upcoming-contests h2 { margin: 0 0 20px; font-size: clamp(1.5rem, 3vw, 2rem); letter-spacing: -.04em; }
.upcoming-contests ul { margin: 0; padding: 0; list-style: none; border-block: 1px solid var(--color-line); background: var(--color-surface); }
.upcoming-contests li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 4px 24px; align-items: baseline; padding: 16px 28px; }
.upcoming-contests li + li { border-top: 1px solid var(--color-line); }
.upcoming-contests time { color: var(--color-muted); font-size: .875rem; font-variant-numeric: tabular-nums; white-space: nowrap; }
.upcoming-contests li a { color: var(--color-ink); font-weight: 650; overflow-wrap: anywhere; }
.upcoming-contests li a:hover { color: var(--color-accent); }
.upcoming-contests li span { color: var(--color-accent); font-size: .8125rem; font-weight: 650; white-space: nowrap; }
.upcoming-more { display: flex; align-items: center; justify-content: flex-end; gap: 20px; width: fit-content; min-height: 44px; margin: 8px 0 0 auto; font-size: .8125rem; text-decoration: none; }
.upcoming-more:hover { text-decoration: underline; }
.upcoming-more:active { transform: translateY(1px); }
@media (max-width: 650px) {
  .upcoming-contests li { grid-template-columns: minmax(0, 1fr) auto; padding: 14px 20px; }
  .upcoming-contests li a { grid-area: 2 / 1 / 3 / -1; }
}
</style>
