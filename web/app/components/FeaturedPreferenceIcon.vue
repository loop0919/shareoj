<script setup lang="ts">
// The rabbit wants an early slot; the tortoise is happy to wait.
defineProps<{ preference: 'soon' | 'later', size?: number }>()
</script>
<template>
  <svg :width="size ?? 22" :height="size ?? 22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <template v-if="preference === 'soon'">
      <path d="M15 10c-2-3-2-7 0-8 2 0 2 4 2 7M18 9c0-3 1-6 3-6 2 1 0 5-1 7M14 10c-2-1-5 0-7 3-2 2-2 5-1 7h13c2 0 2-2 0-2h-2l1-3h2c3 0 3-4 0-5-2-1-4-1-6 0Z" />
      <path d="M9 15c3-1 5 3 2 5M5 14a2 2 0 1 0 0 4" /><circle cx="19" cy="12" r=".6" fill="currentColor" stroke="none" />
    </template>
    <template v-else>
      <path d="M3 15c0-5 3-8 7-8s7 3 7 8H3ZM3 15l-2-1M6 16l-1 3h3l1-3m4 0 1 3h3l-1-3M17 12c1-3 5-3 6 0v3h-6M6 9l4 3 4-3m-4 3v3" />
      <circle cx="20" cy="12" r=".6" fill="currentColor" stroke="none" />
    </template>
  </svg>
</template>
