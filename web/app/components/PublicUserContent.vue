<script setup lang="ts">
import type { z } from 'zod'
import type { publicProblemSummarySchema } from '~~/shared/types/problem'
import type { postSummarySchema } from '~~/shared/types/post'
const props = defineProps<{ handle: string, kind: 'problems' | 'posts' }>()
const endpoint = `/api/${props.kind}` as const
type Item = z.infer<typeof publicProblemSummarySchema> | z.infer<typeof postSummarySchema>
type Page = { items: Item[], nextCursor: string }
const fetchPage = (cursor: string) => $fetch<Page>(endpoint, { query: { author: props.handle, cursor } })
const { data, error, refresh } = await useAsyncData(`user-content:${props.handle}:${props.kind}`, () => fetchPage(''))
const pages = computed(() => data.value)
</script>
<template>
  <div class="draft-library">
    <header class="draft-library-heading"><h2>{{ kind === 'problems' ? '問題' : '記事' }}</h2></header>
    <p v-if="error" role="alert">一覧を取得できませんでした。<button class="editor-button" @click="refresh()">再試行</button></p>
    <UserContentTable v-else-if="pages" :key="`${handle}:${kind}`" :initial="pages" :kind="kind" :fetch-page="fetchPage" />
    <LoadingSpinner v-else />
  </div>
</template>
