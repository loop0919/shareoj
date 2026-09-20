<script setup lang="ts">
import { submissionUsage } from '~/utils/submission-usage'
import { contestDate, contestStatus, type Contest, type Standing } from '~~/shared/types/contest'
import type { Submission } from '~~/shared/types/submission'
definePageMeta({ key: route => String(route.params.id) })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const views = { overview: '概要', problems: '問題', standings: '順位表', submissions: '提出一覧' }
const activeView = computed(() => {
  const view = route.query.view
  return view === 'problems' || view === 'standings' || view === 'submissions' ? view : 'overview'
})
const { user } = useAccount()
const base = `/api/contests/${encodeURIComponent(String(route.params.id))}`
const { data: contest, error, refresh } = await useFetch<Contest>(base)
if (error.value || !contest.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: 'コンテストを取得できませんでした', fatal: true })
useSeoMeta({ title: () => `${contest.value?.title} | ShareOJ` })
const { data: standings, error: standingsError, refresh: refreshStandings } = await useFetch<Standing[]>(`${base}/standings`)
const canViewSubmissions = computed(() => contest.value?.canViewSubmissions === true)
const submissions = ref<{ items: Submission[], hasMore: boolean } | null>(null)
const submissionsError = ref('')
const offset = ref(0)
const { run: requestSubmissions, invalidate: invalidateSubmissions } = useLatestRequest()
function loadSubmissions() {
  if (activeView.value !== 'submissions' || !canViewSubmissions.value) { invalidateSubmissions(); submissions.value = null; return Promise.resolve() }
  const query = { offset: offset.value }
  return requestSubmissions(JSON.stringify([query, user.value?.id]),
    () => $fetch<{ items: Submission[], hasMore: boolean }>(`${base}/submissions`, { query }),
    result => { submissions.value = result; submissionsError.value = '' },
    () => { submissionsError.value = '提出一覧を取得できませんでした。' })
}
const updating = ref(false)
async function update() {
  if (updating.value) return
  updating.value = true
  try { await Promise.all([refresh(), refreshStandings()]); await loadSubmissions() }
  finally { updating.value = false }
}
watch(() => user.value?.id, () => { invalidateSubmissions(); submissions.value = null; void update() })
watch([offset, activeView, canViewSubmissions], () => { void loadSubmissions() })
onMounted(loadSubmissions)
usePolling(update, 15000)
function problemLabel(index: number): string {
  return index < 26 ? String.fromCharCode(65 + index) : problemLabel(Math.floor(index / 26) - 1) + problemLabel(index % 26)
}
function elapsed(acceptedAt: string) { return duration(new Date(acceptedAt).getTime() - new Date(contest.value!.startsAt).getTime()) }
const problemStats = computed(() => contest.value!.problems.map(problem => {
  let submitted = 0
  let accepted = 0
  let firstAt = Infinity
  let handles: string[] = []
  for (const row of standings.value ?? []) {
    const score = row.problems[problem.id]
    if (!score) continue
    submitted++
    if (!score.acceptedAt) continue
    accepted++
    const at = new Date(score.acceptedAt).getTime()
    if (at < firstAt) { firstAt = at; handles = [row.handle] }
    else if (at === firstAt) handles.push(row.handle)
  }
  return { id: problem.id, submitted, accepted, handles, time: accepted ? duration(firstAt - new Date(contest.value!.startsAt).getTime()) : '' }
}))
function duration(ms: number) { const seconds = Math.floor(ms / 1000); return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` }
useSharePreview({ enabled: () => !!contest.value && contest.value.status !== 'draft', type: 'website', title: () => contest.value!.title, path: () => `/contests/${contest.value!.id}`, description: () => contest.value!.description.slice(0, 160) })
</script>
<template>
  <div v-if="contest" class="problem-page contest-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/contests">コンテスト</NuxtLink><span aria-hidden="true">/</span><span>{{ contest.title }}</span></nav><TweetButton v-if="contest.status !== 'draft'" :title="contest.title" :url="`/contests/${contest.id}`" /></div>
    <nav class="problem-menu" aria-label="コンテストメニュー">
      <NuxtLink v-for="(label, view) in views" :key="view" :to="{ path: `/contests/${contest.id}`, query: view === 'overview' ? {} : { view } }" :aria-current="activeView === view ? 'page' : undefined">{{ label }}</NuxtLink>
    </nav>
    <header class="problem-header">
      <div class="contest-heading">
        <h1>{{ contest.title }}</h1>
        <NuxtLink v-if="contest.canEdit" class="editor-button contest-edit" :to="`/my/contests/${contest.id}`"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg>コンテストを編集</NuxtLink>
      </div>
      <div class="problem-summary"><div class="problem-meta muted"><span class="contest-status" :data-status="contest.status">{{ contestStatus[contest.status] }}</span><p>作成者 <UserLink :handle="contest.author" /></p></div></div>
      <dl class="limits contest-schedule"><div><dt>開始日時</dt><dd><time :datetime="contest.startsAt">{{ contestDate(contest.startsAt) }}</time></dd></div><div><dt>終了日時</dt><dd><time :datetime="contest.endsAt">{{ contestDate(contest.endsAt) }}</time></dd></div></dl>
      <p class="schedule-timezone muted">日時は日本時間で表示しています。</p>
      <p class="contest-scoring muted">誤答ペナルティ {{ contest.penaltyMinutes }} 分 · 部分点なし</p>
    </header>
    <p v-if="contest.status === 'draft'" class="notice">このコンテストは未公開です。<NuxtLink to="/contests?post=1">コンテスト一覧の「コンテスト投稿」</NuxtLink>から公開できます。</p>
    <section v-if="activeView === 'overview'" id="overview" class="contest-section contest-description" aria-labelledby="overview-title">
      <h2 id="overview-title">概要</h2><ProblemMarkdown v-if="contest.description" :source="contest.description" /><p v-else class="muted">コンテストの説明はまだありません。</p>
      <p><NuxtLink to="/blog/contest-rules">ルール</NuxtLink></p>
    </section>
    <section v-if="activeView === 'problems'" id="problems" class="contest-section" aria-labelledby="problems-title">
      <h2 id="problems-title">問題</h2>
      <p v-if="contest.status === 'scheduled'" class="notice">問題は開始時刻に公開されます。事前に閲覧できるのは作成者と、その問題のテスターです。</p>
      <div v-if="contest.problems.length" class="content-table-scroll" role="region" aria-label="コンテストの問題" tabindex="0">
        <table class="content-table contest-problems"><thead><tr><th scope="col">#</th><th scope="col">問題</th><th scope="col">配点</th></tr></thead><tbody><tr v-for="(p, index) in contest.problems" :key="p.id" :class="{ solved: p.solved }"><td>{{ problemLabel(index) }}</td><th scope="row"><NuxtLink :to="`/contests/${contest.id}/problems/${p.id}`" :aria-label="p.solved ? `${p.title}（AC 済み）` : undefined">{{ p.title }}</NuxtLink></th><td>{{ p.points }} 点</td></tr></tbody></table>
      </div>
    </section>
    <section v-if="activeView === 'standings'" id="standings" class="contest-section" aria-labelledby="standings-title">
      <header class="contest-section-heading"><h2 id="standings-title">公式順位表</h2><button class="editor-button" :disabled="updating" :aria-busy="updating" @click="update">{{ updating ? '更新中…' : '今すぐ更新' }}</button></header>
      <p v-if="user && !contest.official" class="notice">作成者・テスターとしての提出は公式順位の対象外です。</p>
      <p v-if="standingsError" class="notice notice-error" role="alert">順位表を取得できませんでした。</p>
      <p v-else-if="!standings?.length" class="contest-empty muted">公式順位の対象となる提出はまだありません。</p>
      <div v-else class="content-table-scroll" role="region" aria-label="順位表のスクロール領域" tabindex="0" :aria-busy="updating">
        <table class="content-table standings-table">
          <thead><tr><th scope="col">順位</th><th scope="col">ユーザー</th><th scope="col">得点</th><th scope="col">時間</th><th v-for="(p, i) in contest.problems" :key="p.id" scope="col"><NuxtLink :to="`/contests/${contest.id}/problems/${p.id}`">{{ problemLabel(i) }}</NuxtLink></th></tr></thead>
          <tbody>
            <tr v-for="row in standings" :key="row.handle">
              <td>{{ row.rank }}</td><th scope="row"><UserLink :handle="row.handle" /></th><td class="standing-total">{{ row.points }}</td><td>{{ duration(row.timeMs) }}</td>
              <td v-for="p in contest.problems" :key="p.id">
                <template v-if="row.problems[p.id]">
                  <span class="standing-result">
                    <strong v-if="row.problems[p.id]!.acceptedAt" class="standing-accepted">{{ row.problems[p.id]!.points }}</strong>
                    <span v-else-if="!row.problems[p.id]!.wrong && !row.problems[p.id]!.pending" class="muted">—</span>
                    <span v-if="row.problems[p.id]!.wrong" class="standing-wrong" :aria-label="`誤答 ${row.problems[p.id]!.wrong} 回`">({{ row.problems[p.id]!.wrong }})</span>
                    <svg v-if="row.problems[p.id]!.pending" class="standing-pending" viewBox="0 0 24 24" role="img" aria-label="判定待ち"><path d="M5 3h14M5 21h14M7 3v4l5 5-5 5v4M17 3v4l-5 5 5 5v4" /></svg>
                  </span>
                  <small v-if="row.problems[p.id]!.acceptedAt">{{ elapsed(row.problems[p.id]!.acceptedAt!) }}</small>
                </template>
                <span v-else class="muted">—</span>
              </td>
            </tr>
          </tbody>
          <tfoot>
            <tr><th colspan="4" scope="row">FA（初正解）</th><td v-for="stat in problemStats" :key="stat.id"><template v-if="stat.accepted"><span v-for="handle in stat.handles" :key="handle" class="standing-fa"><UserLink :handle="handle" /></span><small>{{ stat.time }}</small></template><span v-else class="muted">—</span></td></tr>
            <tr><th colspan="4" scope="row">正解者数 / 提出者数</th><td v-for="stat in problemStats" :key="stat.id"><span class="standing-accepted">{{ stat.accepted }}</span> / {{ stat.submitted }}</td></tr>
          </tfoot>
        </table>
      </div>
    </section>
    <section v-if="activeView === 'submissions'" id="submissions" class="contest-section" aria-labelledby="submissions-title">
      <h2 id="submissions-title">提出一覧</h2>
      <p v-if="!canViewSubmissions" class="notice">提出一覧と提出コードはコンテスト終了後に公開されます。終了前はコンテストセッターとテスターが閲覧できます。</p>
      <template v-else><p class="muted">提出コードを閲覧できます。練習提出も掲載します。</p>
      <p v-if="submissionsError" class="notice notice-error" role="alert">{{ submissionsError }}</p>
      <p v-if="!submissions && !submissionsError" class="muted" role="status">提出一覧を読み込み中…</p>
      <template v-if="submissions"><p v-if="!submissions.items.length" class="contest-empty muted">提出はまだありません。</p><div v-else class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0"><table class="content-table"><thead><tr><th scope="col">問題</th><th scope="col">ユーザー</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th scope="col">結果</th><th scope="col">提出日時（日本時間）</th></tr></thead><tbody><tr v-for="s in submissions.items" :key="s.id"><th scope="row"><NuxtLink :to="`/contests/${contest.id}/submissions/${s.id}`">{{ s.problemTitle }}</NuxtLink></th><td><UserLink :handle="s.author" /></td><td class="submission-usage">{{ submissionUsage(s.result) }}</td><td><SubmissionStatus :item="s" /></td><td>{{ contestDate(s.createdAt) }}<span v-if="new Date(s.createdAt) >= new Date(contest.endsAt)"> · 練習</span></td></tr></tbody></table></div><ContentPagination :index="offset / 50" :has-next="submissions.hasMore" :loading="updating" @move="direction => offset += direction * 50" /></template>
      </template>
    </section>
  </div>
</template>
<style scoped>
.submission-usage { white-space: nowrap; font-variant-numeric: tabular-nums; }
/* Hallmark · component: contest standings · genre: modern-minimal · theme: existing ShareOJ tokens
 * pre-emit critique: P4 H4 E4 S5 R5 V4
 * modern-minimal · Long Document · existing problem page tokens and hierarchy */
.contest-page { padding-bottom: 64px; }
.contest-heading { display: flex; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; gap: 16px 24px; margin-bottom: 20px; }
.contest-heading h1 { flex: 1 1 24rem; margin: 0; }
.contest-edit { flex-shrink: 0; min-height: 44px; padding: 8px 16px; font-weight: 600; }
.contest-status { display: inline-flex; align-items: center; min-height: 28px; padding: 2px 10px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); background: var(--color-surface); white-space: nowrap; }
.contest-status[data-status="running"] { color: var(--color-accent); background: var(--color-accent-soft); border-color: var(--color-accent); }
.contest-schedule { gap: 12px 40px; }
.contest-schedule div { flex-wrap: wrap; gap: 4px 12px; }
.schedule-timezone { margin: 8px 0 12px; }
.contest-scoring { margin: 0; }
.contest-section { padding-block: 32px; }
.contest-section > h2, .contest-section-heading { margin: 0 0 20px; }
.contest-description { max-width: 52rem; }
.contest-section-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
.contest-section-heading h2 { margin: 0; }
.contest-section-heading button { min-height: 40px; padding: 8px 12px; }
.contest-section-heading button:disabled { opacity: .55; cursor: wait; }
.contest-problems tr.solved { background: var(--color-accent-soft); }
.contest-problems td:first-child { width: 3rem; color: var(--color-muted); }
.contest-problems thead th:last-child, .contest-problems td:last-child { text-align: right; font-variant-numeric: tabular-nums; }
.contest-empty { margin: 0; padding-block: 24px; border-top: 1px solid var(--color-line); }
.standings-table th, .standings-table td { padding: 8px 12px; border: 1px solid var(--color-line); text-align: center; font-variant-numeric: tabular-nums; }
.standings-table thead, .standings-table tfoot, .standings-table tbody tr:nth-child(even) { background: var(--color-surface); }
.standings-table tbody th { text-align: left; }
.standings-table tfoot th { font-weight: 500; }
.standing-total { font-weight: 700; }
.standing-result { display: inline-flex; align-items: center; justify-content: center; gap: 4px; }
.standing-pending { width: 16px; height: 16px; flex-shrink: 0; fill: none; stroke: currentColor; stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; color: var(--color-muted); }
.standing-accepted, .standing-fa { color: var(--color-accent); }
.standing-wrong { color: var(--color-error); }
.standing-fa { display: block; }
.standings-table small { font-size: .75rem; }
small { display: block; color: var(--color-muted); }
@media (max-width: 39.999rem) { .contest-section-heading { align-items: flex-start; flex-direction: column; } }
</style>
