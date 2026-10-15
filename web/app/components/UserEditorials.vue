<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
import type { UserEditorial } from '~~/shared/types/post'
import { shortId } from '~~/shared/utils/short-id'
const props = defineProps<{ problemId: string }>()
const { data, error } = await useFetch<{ items: UserEditorial[] }>(() => `/api/problems/${props.problemId}/editorials`)
</script>
<template>
  <section class="user-editorials" aria-labelledby="user-editorials-title">
    <header class="user-editorials-heading">
      <h2 id="user-editorials-title">ユーザー解説</h2>
      <NuxtLink class="editor-button" :to="{ path: '/blog/new', query: { problem: problemId } }">ユーザー解説を書く</NuxtLink>
    </header>
    <p v-if="error" class="notice notice-error" role="alert">ユーザー解説を取得できませんでした。</p>
    <p v-else-if="!data?.items.length" class="muted">ユーザー解説はまだありません。</p>
    <ul v-else class="user-editorial-list">
      <li v-for="item in data.items" :key="item.id">
        <NuxtLink :to="`/blog/${shortId(item.id)}`">{{ item.title }}</NuxtLink>
        <span class="muted"><UserLink :handle="item.author" /><span v-if="item.isOperator">・運営</span> · <time :datetime="item.publishedAt">{{ contestDate(item.publishedAt) }}</time></span>
      </li>
    </ul>
  </section>
</template>
<style scoped>
.user-editorials { margin-top: 48px; padding-top: 32px; border-top: 1px solid var(--color-line); }
.user-editorials-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; margin-bottom: 16px; }
.user-editorials-heading h2 { margin: 0; }
.user-editorials-heading a { min-height: 40px; padding: 8px 12px; text-decoration: none; }
.user-editorial-list { list-style: none; padding: 0; margin: 0; }
.user-editorial-list li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 16px; padding: 12px 0; border-bottom: 1px solid var(--color-line); overflow-wrap: anywhere; }
.user-editorial-list li > a { font-weight: 600; }
</style>
