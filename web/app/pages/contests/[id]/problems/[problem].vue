<script setup lang="ts">
import type { Contest, ContestProblemDetail } from '~~/shared/types/contest'
definePageMeta({ key: route => route.path })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const { profile } = useAccount()
const id = encodeURIComponent(String(route.params.id))
const pid = encodeURIComponent(String(route.params.problem))
const { data: contest, refresh: refreshContest } = await useFetch<Contest>(`/api/contests/${id}`)
const { data: problem, error, refresh } = await useFetch<ContestProblemDetail>(`/api/contests/${id}/problems/${pid}`)
if (error.value || !problem.value || !contest.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: '問題が見つからないか、まだ公開されていません', fatal: true })
const showingEditorial = computed(() => route.query.view === 'editorial')
const submissionView = computed(() => route.query.view === 'my-submissions' || route.query.view === 'submissions')
const problemPath = `/contests/${id}/problems/${pid}`
useSeoMeta({ title: () => `${problem.value?.title} | ${contest.value?.title} | ShareOJ` })
usePolling(() => Promise.all([refresh(), refreshContest()]), 15000)
useSharePreview({ title: () => problem.value!.title, path: problemPath, description: () => problem.value!.markdown.slice(0, 160), enabled: () => !!problem.value && !!contest.value && contest.value.status !== 'scheduled' })
</script>
<template>
  <div v-if="problem && contest" class="problem-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink :to="`/contests/${id}?view=problems`">{{ contest.title }}</NuxtLink><span aria-hidden="true">/</span><span>{{ problem.title }}</span></nav><TweetButton v-if="contest.status !== 'scheduled'" :title="problem.title" :url="problemPath" /></div>
    <nav class="problem-menu" aria-label="問題メニュー"><NuxtLink :to="problemPath" :aria-current="showingEditorial || submissionView ? undefined : 'page'">問題</NuxtLink><NuxtLink v-if="contest.status === 'ended' || profile?.handle === problem.author" :to="{ path: problemPath, query: { view: 'editorial' } }" :aria-current="showingEditorial ? 'page' : undefined">解説</NuxtLink><NuxtLink :to="{ path: problemPath, query: { view: 'my-submissions' } }" :aria-current="route.query.view === 'my-submissions' ? 'page' : undefined">自分の提出</NuxtLink><NuxtLink :to="{ path: problemPath, query: { view: 'submissions' } }" :aria-current="route.query.view === 'submissions' ? 'page' : undefined">すべての提出</NuxtLink></nav>
    <header class="problem-header"><h1>{{ problem.title }}</h1><div class="problem-summary"><div class="problem-meta muted"><p>作成者 <UserLink :handle="problem.author" /></p><p v-if="problem.testers?.length">テスター <template v-for="(tester, index) in problem.testers" :key="tester"><span v-if="index">、</span><UserLink :handle="tester" /></template></p><p>配点 {{ contest.problems.find(p => p.id === problem!.id)?.points }} 点</p></div></div><dl class="limits"><div><dt>実行時間制限</dt><dd>{{ Number(problem.timeLimitMs) / 1000 }} 秒</dd></div><div><dt>メモリ制限</dt><dd>{{ problem.memoryLimitMb }} MiB</dd></div></dl></header>
    <template v-if="submissionView">
      <p v-if="route.query.view === 'submissions' && !contest.canViewSubmissions" class="notice submission-notice">すべての提出はコンテスト終了後に公開されます。終了前はコンテストセッターとテスターが閲覧できます。</p>
      <ProblemSubmissionList v-else :key="String(route.query.view)" :problem-id="problem.id" :contest-id="contest.id" :mine="route.query.view === 'my-submissions'" />
    </template>
    <article v-else class="problem-body">
      <template v-if="showingEditorial"><ProblemMarkdown v-if="problem.editorial" :source="problem.editorial" /><p v-else class="muted">解説は終了後に公開されます。終了後も表示されない場合は未登録です。</p></template>
      <template v-else><ProblemMarkdown :source="problem.markdown" /><p v-if="problem.interactive" class="muted">インタラクティブ問題：応答を待つ前に出力をflushしてください。</p><p v-if="problem.specialJudge" class="muted">スペシャルジャッジ問題です。</p><SubmissionForm :has-samples="problem.hasSamples" :problem-id="problem.id" :contest-id="contest.id" /></template>
    </article>
  </div>
</template>

<style scoped>
.submission-notice { margin-block: 32px 64px; }
</style>
