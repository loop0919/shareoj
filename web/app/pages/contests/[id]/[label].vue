<script setup lang="ts">
import { contestAuthors, contestProblemPath, contestProblemTitle, labelIndex, type ContestProblemDetail } from '~~/shared/types/contest'
import { fullId, shortId } from '~~/shared/utils/short-id'
definePageMeta({ key: route => route.path, middleware: 'contest-problem-lists' })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const { user } = useAccount()
const id = fullId(route.params.id)
const index = labelIndex(route.params.label)
const { data: contest, error: contestError, refresh: refreshContest } = await useContest(id, true)
// The contest returns a problem's ID only to viewers who may open it.
const pid = contest.value?.problems[index]?.id
if (!pid) throw createError({ statusCode: contestError.value && contestError.value.statusCode !== 404 ? 502 : 404, statusMessage: '問題が見つからないか、まだ公開されていません', fatal: true })
const { data: problem, error, refresh } = await useFetch<ContestProblemDetail>(`/api/contests/${encodeURIComponent(id)}/problems/${encodeURIComponent(pid)}`)
if (error.value || !problem.value || !contest.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: '問題が見つからないか、まだ公開されていません', fatal: true })
// The editorial has no tab; 解説 links to it (ADR 0034).
const showingEditorial = computed(() => route.query.view === 'editorial')
const problemPath = contestProblemPath(id, index)
const title = computed(() => contestProblemTitle(index, problem.value!.title))
// Staff test the problems at any time; others submit once registered, or as practice after the end. Former staff are
// neither, so they see no form until the end (ADR 0022).
const canSubmit = computed(() => {
  const c = contest.value
  if (!c || c.status === 'ended') return !!c
  return !!user.value && ((c.staff ?? !c.official) || (c.official && c.participating))
})
// A team member's submissions go to the team (ADR 0024).
const team = computed(() => !!contest.value?.team)
useSeoMeta({ title: () => `${title.value} | ${contest.value?.title} | ShareOJ` })
// The viewer's roles decide the form, so it follows a login or logout.
watch(() => user.value?.id, () => { void Promise.all([refresh(), refreshContest()]) })
usePolling(() => Promise.all([refresh(), refreshContest()]), 15000)
useSharePreview({ title: () => title.value, path: problemPath, description: () => problem.value!.markdown.slice(0, 160), enabled: () => !!problem.value && !!contest.value && contest.value.status !== 'scheduled' })
</script>
<template>
  <div v-if="problem && contest" class="problem-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink :to="`/contests/${shortId(id)}`">{{ contest.title }}</NuxtLink><span aria-hidden="true">/</span><NuxtLink v-if="showingEditorial" :to="`/contests/${shortId(id)}?view=editorials`">解説</NuxtLink><NuxtLink v-else :to="`/contests/${shortId(id)}?view=problems`">問題</NuxtLink><span aria-hidden="true">/</span><span>{{ title }}</span></nav><TweetButton v-if="contest.status !== 'scheduled'" :title="title" :url="problemPath" /></div>
    <header class="problem-header"><p v-if="showingEditorial" class="eyebrow editorial-eyebrow">解説</p><h1><ContestProblemTitle :index="index" :title="problem.title" /></h1><div class="problem-summary"><div class="problem-meta muted"><p>作成者 <template v-for="(author, index) in contestAuthors(problem)" :key="author"><span v-if="index">、</span><UserLink :handle="author" /></template></p><p v-if="problem.testers?.length">テスター <template v-for="(tester, index) in problem.testers" :key="tester"><span v-if="index">、</span><UserLink :handle="tester" /></template></p><p v-if="!problem.scoring">配点 {{ contest.problems.find(p => p.id === problem!.id)?.points }} 点</p></div></div><dl class="limits"><div><dt>実行時間制限</dt><dd>{{ Number(problem.timeLimitMs) / 1000 }} 秒</dd></div><div><dt>メモリ制限</dt><dd>{{ problem.memoryLimitMb }} MiB</dd></div></dl><ProblemScoring v-if="problem.scoring" :scoring="problem.scoring" /></header>
    <article class="problem-body">
      <template v-if="showingEditorial"><ProblemMarkdown v-if="problem.editorial" :source="problem.editorial" /><p v-else class="muted">解説は終了後に公開されます。終了後も表示されない場合は未登録です。</p><UserEditorials v-if="contest.status === 'ended'" :problem-id="problem.id" /></template>
      <template v-else><ProblemMarkdown :source="problem.markdown" /><p v-if="problem.interactive" class="muted">インタラクティブ問題：応答を待つ前に出力をflushしてください。</p><p v-if="problem.specialJudge" class="muted">スペシャルジャッジ問題です。</p><ContestParticipation :contest="contest" @joined="value => contest = value" /><SubmissionForm v-if="canSubmit" :has-samples="problem.hasSamples" :problem-id="problem.id" :contest-id="contest.id" :team="team" /></template>
    </article>
  </div>
</template>

<style scoped>
/* Without a tab to mark it, the label above the title says this is the editorial; the text often has its own 解説 heading. */
.editorial-eyebrow { margin: 0; }
.editorial-eyebrow + h1 { margin-top: 4px; }
</style>
