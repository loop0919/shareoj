<script setup lang="ts">
import type { z } from 'zod'
import type { publicProblemListSchema } from '~~/shared/types/problem'
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
const postDialog = ref<{ open: () => Promise<void> }>()
const route = useRoute()
onMounted(() => { if (route.query.post === '1') void postDialog.value?.open() })
const config = useRuntimeConfig()
const canonical = new URL('/problems', config.public.siteUrl).href
const { data, error } = await useFetch('/api/problems')
if (error.value || !data.value) throw createError({ statusCode: 502, statusMessage: 'Problem service unavailable', fatal: true })
const { current, index, loading, message, move } = useContentPages(data.value, cursor => $fetch<z.infer<typeof publicProblemListSchema>>('/api/problems', { query: { cursor } }))
const description = 'ShareOJ のプログラミング問題一覧。ユーザーが作成・公開した問題に挑戦し、コードを提出して自動採点できます。'
useSeoMeta({ title: '問題 | ShareOJ', description, ogDescription: description, ogTitle: '問題 | ShareOJ', ogUrl: canonical, ogType: 'website' })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: '問題', path: '/problems', description })
</script>
<template>
  <div class="catalogue">
    <ProblemPostDialog ref="postDialog" />
    <header class="catalogue-heading"><h1>問題</h1><button type="button" class="editor-button primary" @click="postDialog?.open()">問題投稿</button></header>
    <p v-if="!current.items.length" class="muted">公開された問題はまだありません。</p>
    <div v-else class="content-table-scroll" tabindex="0" role="region" aria-label="問題一覧" :aria-busy="loading">
      <table class="content-table">
        <thead><tr><th scope="col">タイトル</th><th scope="col">作成者</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">正解者数</th><th scope="col"><abbr title="お気に入り数">Fav</abbr></th><th scope="col">難易度</th></tr></thead>
        <tbody><tr v-for="problem in current.items" :key="problem.id" :class="{ solved: solved.has(problem.id) }">
          <th scope="row"><NuxtLink :to="`/problems/${problem.id}`" :aria-label="solved.has(problem.id) ? `${problem.title}（AC 済み）` : undefined">{{ problem.title }}</NuxtLink></th>
          <td><UserLink :handle="problem.author" /></td><td>{{ problem.timeLimitMs == null ? '—' : `${problem.timeLimitMs / 1000} 秒` }}・{{ problem.memoryLimitMb == null ? '—' : `${problem.memoryLimitMb} MiB` }}</td>
          <td>{{ problem.solverCount }}</td><td>{{ problem.favoriteCount }}</td><td><DifficultyBadge :level="problem.difficulty" /></td>
        </tr></tbody>
      </table>
    </div>
    <p v-if="solvedMessage" role="status">{{ solvedMessage }} <button type="button" class="editor-button" @click="loadSolved">再試行</button></p>
    <p v-if="message" role="alert">{{ message }}</p>
    <ContentPagination :index="index" :has-next="!!current.nextCursor" :loading="loading" @move="move" />
  </div>
</template>

<style scoped>
.content-table tr.solved { background: var(--color-accent-soft); }
</style>
