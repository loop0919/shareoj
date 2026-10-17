<script setup lang="ts">
// Reloading is routine, so the icon stays quiet. Two arrows set it apart from the rejudge icon.
withDefaults(defineProps<{ busy?: boolean, label?: string }>(), { label: '更新' })
</script>

<template>
  <button type="button" class="editor-button refresh-button" :aria-busy="busy || undefined" :aria-label="label" :title="label">
    <svg :class="{ spinning: busy }" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" /><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" /><path d="M8 16H3v5" /></svg>
  </button>
</template>

<style scoped>
/* Hallmark · component: icon refresh button · existing green theme
 * states: default · hover · focus (global ring) · active (global press) · disabled · loading
 * pre-emit critique: P4 H4 E4 S4 R5 V4 */
.editor-button.refresh-button { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 40px; min-height: 40px; padding: 0; color: var(--color-muted); }
.editor-button.refresh-button:hover:not(:disabled) { color: var(--color-ink); }
.editor-button.refresh-button:disabled { cursor: progress; transform: none; }
.spinning { animation: refresh-spin .9s linear infinite; }
@keyframes refresh-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .spinning { animation: none; } }
</style>
