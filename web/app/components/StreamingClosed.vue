<script setup lang="ts">
const { askOff } = useStreamingMode()
useSeoMeta({ title: '配信モード中は表示できません | ShareOJ', robots: 'noindex, nofollow' })
</script>

<template>
  <div class="error-page">
    <h1>配信モード中は表示できません</h1>
    <p>このページは、配信に映らないように閉じています。</p>
    <p class="closed-actions">
      <!-- A full page load: the router may still stand on the closed route, so no in-site move starts from here. -->
      <a href="/" class="return-link">ホームへ戻る</a>
      <button type="button" class="return-link" @click="askOff">配信モードを切る</button>
    </p>
  </div>
</template>

<style scoped>
.closed-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; }
button.return-link { padding: 0; border: 0; background: none; font: inherit; cursor: pointer; }
</style>
