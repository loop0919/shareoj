<script setup lang="ts">
useSeoMeta({ title: 'コード実行 | ShareOJ' })
</script>

<template>
  <div class="catalogue code-run-page">
    <header class="catalogue-heading"><h1>コード実行</h1></header>
    <CodeRunner />
  </div>
</template>

<style scoped>
/* A source editor stretched by hand can end the page, so keep it off the footer's rule. */
.code-run-page { padding-bottom: 48px; }
</style>
