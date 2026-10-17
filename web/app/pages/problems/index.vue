<script setup lang="ts">
import type { z } from 'zod'
import type { publicProblemListSchema } from '~~/shared/types/problem'
import { shortId } from '~~/shared/utils/short-id'
// Each tag gets a fresh list and pagination.
definePageMeta({ key: route => `problems:${typeof route.query.tag === 'string' ? route.query.tag : ''}` })
const { user } = useAccount()
const solved = ref(new Set<string>())
const solvedMessage = ref('')
let solvedRequest = 0
async function loadSolved() {
  const version = ++solvedRequest
  solved.value = new Set()
  solvedMessage.value = ''
  if (!user.value) return
  try {
    const result = await $fetch('/api/my/solved-problems')
    if (version === solvedRequest) solved.value = new Set(result.items)
  } catch {
    if (version === solvedRequest) solvedMessage.value = '正解状況を取得できませんでした。'
  }
}
watch(() => user.value?.id, loadSolved)
onMounted(loadSolved)
onBeforeUnmount(() => { solvedRequest++ })
const route = useRoute()
// Older links opened the posting modal here; posting now has its own page.
if (route.query.post === '1') await navigateTo({ path: '/problems/post', query: typeof route.query.problem === 'string' ? { problem: route.query.problem } : {} }, { replace: true, redirectCode: 301 })
const config = useRuntimeConfig()
// Rendering depends on the tag preference cookie.
useResponseHeader('Vary').value = 'Cookie'
const showTags = useProblemTags()
const tag = typeof route.query.tag === 'string' ? route.query.tag : ''
const path = tag ? `/problems?${new URLSearchParams({ tag })}` : '/problems'
const canonical = new URL(path, config.public.siteUrl).href
const { data, error } = await useFetch('/api/problems', { query: tag ? { tag } : {} })
if (error.value?.statusCode === 400) throw createError({ statusCode: 404, statusMessage: 'Tag not found', fatal: true })
if (error.value || !data.value) throw createError({ statusCode: 502, statusMessage: 'Problem service unavailable', fatal: true })
const { current, index, loading, message, move } = useContentPages(data.value, cursor => $fetch<z.infer<typeof publicProblemListSchema>>('/api/problems', { query: tag ? { cursor, tag } : { cursor } }))
const heading = tag ? `タグ「${tag}」の問題` : '問題'
const description = tag ? `ShareOJ で「${tag}」のタグが付いたプログラミング問題の一覧。` : 'ShareOJ のプログラミング問題一覧。ユーザーが作成・公開した問題に挑戦し、コードを提出して自動採点できます。'
useSeoMeta({ title: `${heading} | ShareOJ`, description, ogDescription: description, ogTitle: `${heading} | ShareOJ`, ogUrl: canonical, ogType: 'website' })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: heading, path, description })
</script>
<template>
  <div class="catalogue">
    <header class="catalogue-heading"><h1>問題</h1><NuxtLink class="editor-button primary catalogue-action" to="/problems/post">問題投稿</NuxtLink></header>
    <!-- Viewers who hide problem tags get no tag search, unless a link opened a filtered list. -->
    <TagFilter v-if="showTags || tag" kind="problem" :tag="tag" to="/problems" />
    <EmptyState v-if="!current.items.length" kind="problem" :title="tag ? `「${tag}」のタグが付いた問題はまだありません。` : '公開された問題はまだありません。'" />
    <div v-else class="content-table-scroll" tabindex="0" role="region" aria-label="問題一覧" :aria-busy="loading">
      <table class="content-table">
        <thead><tr><th scope="col">タイトル</th><th scope="col">作成者</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">正解者数</th><th scope="col"><abbr title="お気に入り数">Fav</abbr></th><th scope="col">難易度</th></tr></thead>
        <tbody><tr v-for="problem in current.items" :key="problem.id" :class="{ solved: solved.has(problem.id) }">
          <th scope="row"><NuxtLink :to="`/problems/${shortId(problem.id)}`" :aria-label="solved.has(problem.id) ? `${problem.title}（AC 済み）` : undefined">{{ problem.title }}</NuxtLink><TagLinks v-if="showTags && problem.tags.length" class="problem-row-tags" :tags="problem.tags" to="/problems" /></th>
          <td><UserLink :handle="problem.author" /></td><td>{{ problem.timeLimitMs == null ? '—' : `${problem.timeLimitMs / 1000} 秒` }}・{{ problem.memoryLimitMb == null ? '—' : `${problem.memoryLimitMb} MiB` }}</td>
          <td>{{ problem.solverCount }}</td><td>{{ problem.favoriteCount }}</td><td><DifficultyBadge :level="problem.difficulty" /></td>
        </tr></tbody>
      </table>
    </div>
    <p v-if="solvedMessage" role="status">{{ solvedMessage }} <button type="button" class="editor-button" @click="loadSolved">再試行</button></p>
    <p v-if="message" role="alert">{{ message }}</p>
    <ContentPagination v-if="current.items.length || index" :index="index" :has-next="!!current.nextCursor" :loading="loading" @move="move" />
  </div>
</template>

<style scoped>
.catalogue-action { min-height: 40px; padding-inline: 16px; }
.content-table tr.solved { background: var(--color-accent-soft); }
.problem-row-tags { margin-top: 6px; }
</style>
