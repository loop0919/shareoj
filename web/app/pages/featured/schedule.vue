<script setup lang="ts">
useResponseHeader('Cache-Control').value = 'no-store'
useSeoMeta({ title: '定期便の予定 | ShareOJ', description: 'ShareOJ 定期便のこれからの出題予定。次回から8回分のEasy・Hardに、応募した作者と難易度を表示します。' })
const { data, error, status, refresh } = await useFetch('/api/featured/schedule')
usePolling(() => refresh(), 60000)
</script>

<template>
  <div class="catalogue featured-schedule-page">
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/featured">定期便</NuxtLink><span aria-hidden="true">/</span><span>予定</span></nav>
    <header class="catalogue-heading">
      <h1>定期便の予定</h1>
      <NuxtLink class="editor-button primary" to="/featured/apply">新作を応募</NuxtLink>
    </header>
    <p v-if="error" class="notice notice-error" role="alert">予定を取得できませんでした。<button class="editor-button" :disabled="status === 'pending'" @click="refresh()">再試行</button></p>
    <template v-else-if="data">
      <p class="schedule-waiting"><span>出題待ちの新作</span><strong>Easy <b>{{ data.waiting.easy }}</b>問</strong><strong>Hard <b>{{ data.waiting.hard }}</b>問</strong></p>
      <div :aria-busy="status === 'pending'"><FeaturedSchedule :plan="data" /></div>
      <p class="schedule-note">毎週月曜・木曜の23時（日本時間）に出題します。次回は予告と同じで確定、2回目以降は今の応募をもとにした見込みです。取り下げや新しい応募で入れ替わります。</p>
    </template>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens (Plain · modern-minimal)
 * One table, the waiting count in the delivery block's mono numerals, no cards. States: loading, error, refresh. */
.featured-schedule-page > .breadcrumb { padding-top: 0; }
.featured-schedule-page > div, .schedule-note { max-width: 46rem; }
.schedule-waiting { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 16px; margin: 0 0 16px; font-size: .8125rem; }
.schedule-waiting > span { color: var(--color-muted); font-size: .75rem; }
.schedule-waiting strong { font-weight: 500; white-space: nowrap; }
.schedule-waiting b { margin-inline: 4px 2px; font-family: var(--font-code); font-size: 1.375rem; font-weight: 600; }
.schedule-note { margin-top: 16px; color: var(--color-muted); font-size: .8125rem; line-height: 1.8; }
.notice-error .editor-button { margin-left: 12px; }
</style>
