<script setup lang="ts">
// Block spinners stand in for a list or panel; inline ones sit beside text or buttons.
withDefaults(defineProps<{ label?: string, inline?: boolean }>(), { label: '読み込み中', inline: false })
</script>
<template>
  <span class="loading-spinner" :class="{ inline }" role="status"><span class="loading-ring" aria-hidden="true" /><span class="loading-label">{{ label }}</span></span>
</template>
<style scoped>
.loading-spinner { display: flex; justify-content: center; padding-block: 24px; color: var(--color-accent); }
.loading-spinner.inline { display: inline-flex; padding-block: 0; vertical-align: middle; }
.loading-ring { width: 24px; height: 24px; border: 2.5px solid var(--color-line); border-top-color: currentColor; border-radius: 50%; animation: loading-spin .8s linear infinite; }
.inline .loading-ring { width: max(1em, 12px); height: max(1em, 12px); border-width: 2px; }
.loading-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
@keyframes loading-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .loading-ring { animation: none; } }
</style>
