<script setup lang="ts">
import type { Submission } from '../../shared/types/submission'
import { formatScore } from '~/utils/score'

// showScore adds the score of a partially scored result where no column shows it (ADR 0025).
const { item, showScore = false } = defineProps<{ item: Submission, showScore?: boolean }>()
const tooltipId = useId()
const pending = computed(() => item.status !== 'DONE')
const verdict = computed(() => pending.value ? item.progress?.verdict : item.result?.verdict)
const label = computed(() => {
  if (!pending.value) return item.result?.verdict ?? '完了'
  if (item.status === 'RUNNING' && item.progress?.phase === 'JUDGING') return `${verdict.value ? verdict.value + ' ' : ''}${item.progress.completed}/${item.progress.total}`
  return 'WJ'
})
</script>

<template>
  <span class="judge-status">
    <span class="verdict-badge" :data-verdict="verdict" :tabindex="pending ? 0 : undefined" :aria-describedby="pending ? tooltipId : undefined">
      <span v-if="pending" class="judge-spinner" aria-hidden="true" />{{ label }}
    </span>
    <span v-if="pending" :id="tooltipId" class="judge-tooltip" role="tooltip">ジャッジ中</span>
    <span v-else-if="showScore && item.result?.scoring" class="judge-score" :data-full="item.result.scoring.score === item.result.scoring.maxScore || undefined">{{ formatScore(item.result.scoring.score) }}点</span>
  </span>
</template>

<style scoped>
.judge-status { position: relative; display: inline-flex; align-items: center; gap: 6px; }
.judge-score { font-variant-numeric: tabular-nums; white-space: nowrap; }
.judge-score[data-full] { color: var(--color-accent); font-weight: 600; }
.verdict-badge { gap: 6px; }
.verdict-badge:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 3px; }
.judge-spinner { width: 12px; height: 12px; border: 2px solid currentColor; border-right-color: transparent; border-radius: 50%; animation: judge-spin .8s linear infinite; }
.judge-tooltip { display: none; position: absolute; bottom: 100%; left: 50%; transform: translateX(-50%); padding: 4px 8px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); white-space: nowrap; font-size: .75rem; z-index: 1; }
.judge-status:hover .judge-tooltip, .judge-status:focus-within .judge-tooltip { display: block; }
@keyframes judge-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .judge-spinner { animation: none; } }
</style>
