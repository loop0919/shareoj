<script setup lang="ts">
import { issueMessage, problemStatus, type ProblemStatusInput } from '~/utils/problem-readiness'
// The editor lists the reasons itself; the library opens them in place.
const props = withDefaults(defineProps<{ problem: ProblemStatusInput, expandable?: boolean }>(), { expandable: true })
const status = computed(() => problemStatus(props.problem))
</script>

<template>
  <details v-if="expandable && status.issues.length" class="problem-status-details" :data-kind="status.kind">
    <summary class="problem-status" :aria-label="`${status.summary}。理由を表示`">
      <svg class="status-mark" viewBox="0 0 24 24" aria-hidden="true">
        <path v-if="status.kind === 'featured'" d="M12 4 2 20h20L12 4Zm0 6v5m0 3h.01" />
        <path v-else d="M6 6l12 12M18 6 6 18" />
      </svg>
      {{ status.label }}
    </summary>
    <ul class="status-issues">
      <li v-for="code in status.issues" :key="code">{{ issueMessage(code, status.kind === 'featured' ? 'featured' : undefined) }}</li>
    </ul>
  </details>
  <span v-else class="problem-status" :data-kind="status.kind" :title="status.summary">
    <svg v-if="status.kind === 'ready'" class="status-mark" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>
    <svg v-else-if="status.issues.length" class="status-mark" viewBox="0 0 24 24" aria-hidden="true"><path :d="status.kind === 'featured' ? 'M12 4 2 20h20L12 4Zm0 6v5m0 3h.01' : 'M6 6l12 12M18 6 6 18'" /></svg>
    <span class="visually-hidden">{{ status.summary }}</span>
    <span aria-hidden="true">{{ status.label }}</span>
  </span>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens; one status per problem, reasons open in place */
.problem-status { display: inline-flex; align-items: center; gap: 4px; min-height: var(--editor-control-size, 32px); padding: 2px 10px; border: 1px solid var(--color-line); border-radius: 999px; font-size: .75rem; white-space: nowrap; }
.status-mark { width: 14px; height: 14px; flex-shrink: 0; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
[data-kind="ready"].problem-status { border-color: var(--color-accent); color: var(--color-accent); }
[data-kind="incomplete"] .problem-status, [data-kind="incomplete"].problem-status { color: var(--color-muted); }
[data-kind="contest"].problem-status, [data-kind="featured"].problem-status, [data-kind="published"].problem-status { border-color: var(--color-accent-soft); background: var(--color-accent-soft); color: var(--color-ink); }
[data-kind="featured"] .problem-status, [data-kind="featured"].problem-status:has(.status-mark) { border-color: var(--color-error); color: var(--color-error); }
summary.problem-status { cursor: pointer; list-style: none; }
summary.problem-status::-webkit-details-marker { display: none; }
summary.problem-status:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.problem-status-details[open] summary.problem-status { background: var(--color-surface); }
.status-issues { margin: 6px 0 0; padding-left: 1.2em; max-width: 22rem; color: var(--color-ink); font-size: .75rem; line-height: 1.6; white-space: normal; overflow-wrap: anywhere; }
.visually-hidden { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
</style>
