<script setup lang="ts">
import { contestDate, contestStatus } from '~~/shared/types/contest'
const props = defineProps<{ id: string }>()
const { data: contest } = useContest(() => props.id)
// A same-day end shows only its time: 2026/09/13 18:58 〜 19:00.
const period = computed(() => {
  if (!contest.value) return null
  const start = contestDate(contest.value.startsAt)
  const end = contestDate(contest.value.endsAt)
  const day = `${start.split(' ')[0]} `
  return { start, end: end.startsWith(day) ? end.slice(day.length) : end }
})
</script>
<template>
  <div v-if="contest && period" class="contest-header-title">
    <NuxtLink class="contest-header-name" :to="`/contests/${contest.id}`"><span>{{ contest.title }}</span></NuxtLink>
    <span class="contest-header-status" :data-status="contest.status">{{ contestStatus[contest.status] }}</span>
    <span class="contest-header-period"><time :datetime="contest.startsAt">{{ period.start }}</time> 〜 <time :datetime="contest.endsAt">{{ period.end }}</time></span>
  </div>
</template>
<style scoped>
.contest-header-title { display: flex; align-items: center; flex-wrap: wrap; gap: 0 12px; min-width: 0; }
.contest-header-name { min-width: 0; color: var(--color-ink); font-weight: 700; text-decoration: none; }
.contest-header-name span { overflow: hidden; text-overflow: ellipsis; }
.contest-header-name:hover span { text-decoration: underline; }
.contest-header-status { flex-shrink: 0; padding: 0 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); background: var(--color-surface); font-size: .75rem; line-height: 1.75; }
.contest-header-status[data-status="running"] { color: var(--color-accent); background: var(--color-accent-soft); border-color: var(--color-accent); }
.contest-header-period { color: var(--color-muted); font-size: .8125rem; font-variant-numeric: tabular-nums; white-space: nowrap; }
</style>
