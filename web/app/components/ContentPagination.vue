<script setup lang="ts">
defineProps<{ index: number, hasNext: boolean, loading: boolean }>()
defineEmits<{ move: [direction: -1 | 1] }>()
</script>
<template>
  <nav class="content-pagination" aria-label="ページネーション">
    <button class="editor-button" :disabled="loading || index === 0" @click="$emit('move', -1)">前へ</button>
    <span class="pagination-status"><span role="status">{{ index + 1 }} ページ</span><LoadingSpinner v-if="loading" inline /></span>
    <button class="editor-button" :disabled="loading || !hasNext" @click="$emit('move', 1)">次へ</button>
  </nav>
</template>
<style scoped>
.content-pagination { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 16px; margin-block: 24px; font-size: .875rem; }
.content-pagination button { white-space: nowrap; }
.pagination-status { display: inline-flex; align-items: center; gap: 8px; }
</style>
