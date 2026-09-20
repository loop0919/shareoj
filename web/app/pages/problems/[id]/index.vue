<script setup lang="ts">
definePageMeta({ key: route => String(route.params.id) })
const route = useRoute()
const config = useRuntimeConfig()
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const { data: problem, error } = await useFetch(() => `/api/problems/${encodeURIComponent(String(route.params.id))}`)
if (error.value || !problem.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: error.value?.statusCode === 404 ? 'Problem not found' : 'Problem service unavailable', fatal: true })
const votes = ref({ average: problem.value.difficultyAverage, count: problem.value.difficultyVoteCount })
function updateVote(average: number | null, count: number) {
  votes.value = { average, count }
}
const showingEditorial = computed(() => route.query.view === 'editorial')
const submissionView = computed(() => route.query.view === 'my-submissions' || route.query.view === 'submissions')
useSeoMeta({ robots: () => problem.value?.isPrivate ? 'noindex, nofollow' : undefined })
const canonical = computed(() => new URL(`/problems/${problem.value!.id}`, config.public.siteUrl).href)
useSeoMeta({ title: () => `${showingEditorial.value ? '解説 | ' : ''}${problem.value?.title} | ShareOJ`, description: () => (showingEditorial.value ? problem.value?.editorial : problem.value?.markdown)?.slice(0, 160), ogTitle: () => `${problem.value?.title} | ShareOJ`, ogUrl: () => canonical.value, ogType: 'article' })
useHead(() => ({ link: [{ rel: 'canonical', href: canonical.value }] }))
useSharePreview({ title: () => problem.value!.title, path: () => `/problems/${problem.value!.id}`, description: () => problem.value!.markdown.slice(0, 160), enabled: () => !problem.value!.isPrivate })
</script>
<template>
  <div v-if="problem" class="problem-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink :to="problem.isPrivate ? '/my' : '/problems'">{{ problem.isPrivate ? 'マイページ' : '問題' }}</NuxtLink><span aria-hidden="true">/</span><span>{{ problem.title }}</span></nav><TweetButton v-if="!problem.isPrivate" :title="problem.title" :url="canonical" /></div>
    <nav class="problem-menu" aria-label="問題メニュー">
      <NuxtLink :to="`/problems/${problem.id}`" :aria-current="showingEditorial || submissionView ? undefined : 'page'">問題</NuxtLink>
      <NuxtLink :to="{ path: `/problems/${problem.id}`, query: { view: 'editorial' } }" :aria-current="showingEditorial ? 'page' : undefined">解説</NuxtLink>
      <NuxtLink :to="{ path: `/problems/${problem.id}`, query: { view: 'my-submissions' } }" :aria-current="route.query.view === 'my-submissions' ? 'page' : undefined">自分の提出</NuxtLink>
      <NuxtLink :to="{ path: `/problems/${problem.id}`, query: { view: 'submissions' } }" :aria-current="route.query.view === 'submissions' ? 'page' : undefined">すべての提出</NuxtLink>
    </nav>
    <header class="problem-header">
      <h1>{{ problem.title }}</h1>
      <p v-if="problem.isPrivate" class="muted">非公開 · 作成者とテスターが閲覧・提出できます。</p>
      <div class="problem-summary">
        <div class="problem-meta muted">
          <p>作成者 <UserLink :handle="problem.author" /></p><p v-if="problem.testers?.length">テスター <template v-for="(tester, index) in problem.testers" :key="tester"><span v-if="index">、</span><UserLink :handle="tester" /></template></p>
          <p>難易度（作成者設定） <DifficultyBadge :level="problem.difficulty" /></p>
          <p v-if="!problem.isPrivate">難易度（投票） <strong>{{ votes.average === null ? '未投票' : `Lv.${votes.average.toFixed(1)}` }}</strong> <span>（{{ votes.count }}票）</span></p>
        </div>
        <div v-if="!problem.isPrivate" class="problem-actions">
          <ProblemFavorite :problem-id="problem.id" :count="problem.favoriteCount" />
          <ProblemDifficultyVote :problem-id="problem.id" :average="votes.average" :count="votes.count" :distribution="problem.difficultyDistribution" @updated="updateVote" />
        </div>
      </div>
      <dl class="limits"><div><dt>実行時間制限</dt><dd>{{ problem.timeLimitMs / 1000 }} 秒</dd></div><div><dt>メモリ制限</dt><dd>{{ problem.memoryLimitMb }} MiB</dd></div></dl>
    </header>
    <ProblemSubmissionList v-if="submissionView" :key="String(route.query.view)" :problem-id="problem.id" :mine="route.query.view === 'my-submissions'" />
    <article v-else class="problem-body" aria-label="問題詳細">
      <template v-if="showingEditorial">
        <ProblemMarkdown v-if="problem.editorial" :source="problem.editorial" />
        <p v-else class="muted">解説はまだありません。</p>
      </template>
      <template v-else>
        <ProblemMarkdown :source="problem.markdown" />
        <p v-if="problem.interactive" class="muted">インタラクティブ問題：標準入出力でジャッジと対話します。応答を待つ前に出力をflushしてください。</p>
        <p v-if="problem.specialJudge" class="muted">スペシャルジャッジ問題：提出の出力を検証コードで判定します。</p>
        <SubmissionForm :problem-id="problem.id" />
      </template>
    </article>
  </div>
</template>

<style scoped>
.problem-summary { align-items: flex-start; flex-wrap: nowrap; }
.problem-meta { flex: 1; padding-top: 8px; }
.problem-actions { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 12px; max-width: 100%; margin-left: auto; }
.problem-actions :deep(.favorite-control) { margin: 0; max-width: 100%; overflow-wrap: anywhere; }
@media (max-width: 600px) { .problem-summary { flex-wrap: wrap; } .problem-meta { flex-basis: 100%; } }
</style>
