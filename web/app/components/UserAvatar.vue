<script setup lang="ts">
withDefaults(defineProps<{ handle: string, avatar?: string, size?: number }>(), { avatar: '', size: 80 })
</script>
<template>
  <span class="user-avatar" :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${size * .42}px` }" aria-hidden="true">
    <img v-if="avatar" :src="avatar" alt="" :width="size" :height="size">
    <span v-else :data-initial="handle.slice(0, 1).toUpperCase() || '?'" />
  </span>
</template>
<style scoped>
.user-avatar { display: inline-flex; flex: none; align-items: center; justify-content: center; border-radius: 50%; overflow: hidden; background: var(--color-accent-soft); color: var(--color-accent); font-weight: 650; line-height: 1; }
.user-avatar img { width: 100%; height: 100%; object-fit: cover; }
.user-avatar [data-initial]::before { content: attr(data-initial); }
</style>
