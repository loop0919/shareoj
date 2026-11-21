<script setup lang="ts">
import { contestCountdown, contestDate, contestRemaining, type ScheduledContest } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'

// running lists contests in progress by their end; otherwise the list is of contests about to start.
// more is the link to every contest, which the last of the stacked sections carries.
const props = defineProps<{ items: ScheduledContest[], now: number, running?: boolean, more?: boolean }>()
const titleId = useId()
const rows = computed(() => props.items.map(contest => props.running
  ? { contest, at: contest.endsAt, date: `${contestDate(contest.endsAt)} まで`, left: contestRemaining(contest.endsAt, props.now) }
  : { contest, at: contest.startsAt, date: contestDate(contest.startsAt), left: contestCountdown(contest.startsAt, props.now) }))
</script>

<template>
  <section class="upcoming-contests" :class="{ 'upcoming-contests--stacked': !more }" :aria-labelledby="titleId">
    <h2 :id="titleId">{{ running ? '開催中のコンテスト' : 'もうすぐ開催のコンテスト' }}</h2>
    <ul>
      <li v-for="{ contest, at, date, left } in rows" :key="contest.id">
        <time :datetime="at">{{ date }}</time>
        <NuxtLink :to="`/contests/${shortId(contest.id)}`">{{ contest.title }}</NuxtLink>
        <span>{{ left }}</span>
      </li>
    </ul>
    <NuxtLink v-if="more" class="upcoming-more" to="/contests">コンテスト一覧<span aria-hidden="true">→</span></NuxtLink>
  </section>
</template>

<style scoped>
.upcoming-contests { margin-block: 24px 48px; padding-top: 28px; border-top: 2px solid var(--color-accent); }
/* Without the link below it, the section sits closer to the one that follows. */
.upcoming-contests--stacked { margin-bottom: 40px; }
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
