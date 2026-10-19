<script setup lang="ts">
import type { ProblemScoringSummary } from '~~/shared/types/problem'
// What readers learn of a partially scored problem (ADR 0025): its full score and each group's points.
defineProps<{ scoring: ProblemScoringSummary }>()
</script>

<template>
  <div class="problem-scoring">
    <p class="problem-scoring-total">部分点あり（満点{{ scoring.fullScore }}点）</p>
    <ul v-if="scoring.groups.length" class="problem-scoring-groups" aria-label="グループの配点">
      <li v-for="(group, index) in scoring.groups" :key="index"><span>{{ group.name }}</span><span class="problem-scoring-points">{{ group.points }}点</span></li>
    </ul>
  </div>
</template>

<style scoped>
/* Hallmark · component: problem scoring summary · existing ShareOJ tokens · sits under the limits like them */
.problem-scoring { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 16px; margin-top: 12px; font-size: .8125rem; }
.problem-scoring-total { margin: 0; font-weight: 600; }
.problem-scoring-groups { display: flex; flex-wrap: wrap; gap: 4px 8px; margin: 0; padding: 0; list-style: none; }
.problem-scoring-groups li { display: inline-flex; gap: 6px; padding: 1px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-surface); overflow-wrap: anywhere; }
.problem-scoring-points { color: var(--color-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
</style>
