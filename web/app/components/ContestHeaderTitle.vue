<script setup lang="ts">
import { contestStatus } from '~~/shared/types/contest'
const props = defineProps<{ id: string }>()
const { data: contest } = useContest(() => props.id)
</script>
<template>
  <NuxtLink v-if="contest" class="contest-header-title" :to="`/contests/${contest.id}`">
    <span class="contest-header-name">{{ contest.title }}</span>
    <span class="contest-header-status" :data-status="contest.status">{{ contestStatus[contest.status] }}</span>
  </NuxtLink>
</template>
<style scoped>
.contest-header-title { gap: 12px; min-width: 0; color: var(--color-ink); text-decoration: none; }
.contest-header-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; font-weight: 700; }
.contest-header-title:hover .contest-header-name { text-decoration: underline; }
.contest-header-status { flex-shrink: 0; padding: 0 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); background: var(--color-surface); font-size: .75rem; line-height: 1.75; }
.contest-header-status[data-status="running"] { color: var(--color-accent); background: var(--color-accent-soft); border-color: var(--color-accent); }
</style>
