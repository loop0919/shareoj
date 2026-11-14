<script setup lang="ts">
defineProps<{ level: number | null }>()
</script>
<template>
  <span v-if="level" class="difficulty" :data-level="level"><svg v-if="level === 10" class="difficulty-crown" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6l5 4 4-7 4 7 5-4-2 12H5Z" /><path d="M5 21h14" /></svg><span v-else class="difficulty-dot" aria-hidden="true" />Lv.{{ level }}</span>
  <span v-else class="muted">未設定</span>
</template>
