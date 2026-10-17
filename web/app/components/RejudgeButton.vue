<script setup lang="ts">
// Rejudging can change results other people see, so the button reads as a cautious red action.
// The icon spins while the request is in flight.
defineProps<{ busy?: boolean }>()
</script>

<template>
  <button type="button" class="editor-button danger rejudge-button" :aria-busy="busy || undefined">
    <svg class="rejudge-icon" :class="{ spinning: busy }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36L21 8" /><path d="M21 3v5h-5" /></svg>
    <slot>リジャッジ</slot>
  </button>
</template>

<style scoped>
/* Hallmark · component: rejudge button · existing green theme, error token for caution
 * states: default · hover · focus (global ring) · active (global press) · disabled · loading
 * pre-emit critique: P4 H4 E4 S4 R5 V4 */
.rejudge-button { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding-inline: 12px; }
.rejudge-button:hover:not(:disabled) { background: var(--color-error-soft); }
.rejudge-button:disabled { opacity: .5; cursor: not-allowed; transform: none; }
.rejudge-icon { flex-shrink: 0; }
.spinning { animation: rejudge-spin .9s linear infinite; }
@keyframes rejudge-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .spinning { animation: none; } }
</style>
