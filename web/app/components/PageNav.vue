<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { pageNumbers } from '~/utils/pagination'
// Page numbers as links that look like buttons, so search engines follow them too (ADR 0028). The neighbours are always
// listed, so there is no previous or next; a single page shows nothing. RouterLink ignores the query and would mark every
// number as the current page, so the links clear aria-current and the current page is plain text.
defineProps<{ page: number, last: number, link: (page: number) => RouteLocationRaw }>()
</script>
<template>
  <nav v-if="last > 1" class="page-nav" aria-label="ページ"><template v-for="number in pageNumbers(page, last)" :key="number"><span v-if="number === page" class="editor-button" aria-current="page">{{ number }}</span><NuxtLink v-else class="editor-button" :to="link(number)" :aria-current="undefined">{{ number }}</NuxtLink></template></nav>
</template>
<style scoped>
.page-nav { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px; }
.page-nav .editor-button { display: inline-flex; align-items: center; justify-content: center; min-width: var(--editor-control-size); font-variant-numeric: tabular-nums; }
.page-nav [aria-current="page"] { border-color: var(--color-accent); background: var(--color-accent); color: var(--color-paper); font-weight: 600; cursor: default; transform: none; }
</style>
