<script setup lang="ts">
// Each tag opens the list of problems or posts that carry it.
defineProps<{ tags: string[], to: '/problems' | '/blog' }>()
</script>
<template>
  <ul class="tag-links" aria-label="タグ">
    <li v-for="tag in tags" :key="tag"><NuxtLink :to="{ path: to, query: { tag } }">{{ tag }}</NuxtLink></li>
  </ul>
</template>
<style scoped>
.tag-links { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
.tag-links a { display: inline-block; padding: 2px 10px; border: 1px solid var(--color-line); border-radius: 999px; background: var(--color-surface); color: var(--color-ink); font-size: .75rem; font-weight: 400; line-height: 1.6; text-decoration: none; overflow-wrap: anywhere; }
.tag-links a:hover { border-color: var(--color-accent); color: var(--color-accent); }
.tag-links a:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
</style>
