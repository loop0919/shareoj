<script setup lang="ts">
const { askOff } = useStreamingMode()
</script>

<template>
  <button type="button" class="streaming-badge" aria-haspopup="dialog" @click="askOff">配信モード</button>
</template>

<style scoped>
.streaming-badge { flex-shrink: 0; min-height: 32px; padding: 0 10px; border: 1px solid var(--color-error); border-radius: 999px; background: light-dark(#fff3f2, #422a2a); color: var(--color-error); font: inherit; font-size: .8125rem; font-weight: 600; white-space: nowrap; cursor: pointer; }
.streaming-badge:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
</style>
