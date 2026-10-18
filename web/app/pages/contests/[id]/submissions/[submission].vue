<script setup lang="ts">
import { submissionProblemPath, type Submission } from '~~/shared/types/submission'
import { fullId, shortId } from '~~/shared/utils/short-id'
definePageMeta({ key: route => route.path })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const id = fullId(route.params.id)
const { data: item, error, refresh } = await useFetch<Submission>(`/api/contests/${encodeURIComponent(id)}/submissions/${encodeURIComponent(fullId(route.params.submission))}`)
if (error.value || !item.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: '提出が見つからないか、まだ公開されていません', fatal: true })
useSeoMeta({ title: () => `提出 | ${item.value?.problemTitle} | ShareOJ`, robots: 'noindex, nofollow' })
// Follow a submission that is still being judged, such as one just rejudged.
usePolling(refresh, 2000, () => !!item.value && item.value.status !== 'DONE')
</script>
<template>
  <div v-if="item" class="submission-page">
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink v-if="route.query.from === 'team'" :to="`${submissionProblemPath(item, id)}?view=my-submissions`">この問題のチームの提出</NuxtLink><NuxtLink v-else-if="route.query.from === 'problem'" :to="`${submissionProblemPath(item, id)}?view=submissions`">この問題のすべての提出</NuxtLink><NuxtLink v-else :to="`/contests/${shortId(id)}?view=submissions`">コンテストの提出一覧</NuxtLink><span aria-hidden="true">/</span><span>提出結果</span></nav>
    <nav class="problem-menu" aria-label="提出メニュー"><NuxtLink :to="submissionProblemPath(item, id)">問題</NuxtLink><NuxtLink :to="`${submissionProblemPath(item, id)}?view=submissions`">すべての提出</NuxtLink></nav>
    <SubmissionDetail :item="item" @rejudged="refresh" />
  </div>
</template>
