<script setup lang="ts">
import { submissionUsage } from '~/utils/submission-usage'
import { contestAuthors, contestDate, contestProblemPath, contestStatus, contestView, contestViews, problemLabel, type Standing } from '~~/shared/types/contest'
import { fullId, shortId } from '~~/shared/utils/short-id'
import type { Submission } from '~~/shared/types/submission'
import type { UserEditorial } from '~~/shared/types/post'
definePageMeta({ key: route => route.path })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
// The layout header shows the contest menu that switches these views.
const activeView = computed(() => contestView(route.query.view))
const { user } = useAccount()
const { showToast } = useToast()
const id = fullId(route.params.id)
const base = `/api/contests/${encodeURIComponent(id)}`
const { data: contest, error, refresh } = await useContest(id)
if (error.value || !contest.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: 'コンテストを取得できませんでした', fatal: true })
useSeoMeta({ title: () => `${contest.value?.title} | ShareOJ` })
// Re-read on each poll so the banner appears once the freeze starts.
const now = ref(Date.now())
const frozen = computed(() => {
  const at = contest.value?.freezeAt
  return !!at && !contest.value?.resultsFinal && (contest.value?.status === 'ended' || now.value >= Date.parse(at))
})
// Staff may switch to the table without the freeze while it hides something; everyone else always gets the frozen one.
const canSwitchStandings = computed(() => contest.value?.canViewLiveStandings === true && frozen.value)
const standingsMode = ref<'frozen' | 'live'>('frozen')
const showLive = computed(() => standingsMode.value === 'live' && canSwitchStandings.value)
const standingsModes = [{ value: 'frozen' as const, label: '凍結した順位表' }, { value: 'live' as const, label: '凍結なし' }]
// The rows carry the view they were read for. While a switch loads, the other view's rows are hidden, so the live table
// never sits under the frozen notice (ADR 0023) nor the frozen one under the live notice.
const requestFetch = useRequestFetch()
const { data: standingsData, error: standingsError, refresh: refreshStandings } = await useAsyncData(`contest-standings:${id}`, async (_, { signal }) => {
  const live = showLive.value
  return { live, rows: await requestFetch<Standing[]>(`${base}/standings`, { query: live ? { live: '1' } : {}, signal }) }
}, { watch: [showLive] })
const switchingStandings = computed(() => !!standingsData.value && standingsData.value.live !== showLive.value)
const standings = computed(() => switchingStandings.value ? undefined : standingsData.value?.rows)
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
// User editorials exist only after the end, when the problems become public.
const userEditorials = ref<UserEditorial[] | null>(null)
const userEditorialsError = ref('')
const { run: requestEditorials, invalidate: invalidateEditorials } = useLatestRequest()
function loadEditorials() {
  if (activeView.value !== 'editorials' || contest.value?.status !== 'ended') { invalidateEditorials(); userEditorials.value = null; return Promise.resolve() }
  return requestEditorials(base,
    () => $fetch<{ items: UserEditorial[] }>(`${base}/editorials`),
    result => { userEditorials.value = result.items; userEditorialsError.value = '' },
    () => { userEditorialsError.value = 'ユーザー解説を取得できませんでした。' })
}
const editorialsByProblem = computed(() => {
  const groups: Record<string, UserEditorial[]> = {}
  for (const item of userEditorials.value ?? []) (groups[item.problemId] ??= []).push(item)
  return groups
})
const updating = ref(false)
async function update() {
  if (updating.value) return
  updating.value = true
  now.value = Date.now()
  try { await Promise.all([refresh(), refreshStandings()]); await Promise.all([loadSubmissions(), loadEditorials()]) }
  finally { updating.value = false }
}
watch(() => user.value?.id, () => { invalidateSubmissions(); submissions.value = null; void update() })
watch([offset, activeView, canViewSubmissions], () => { void loadSubmissions() })
watch([activeView, () => contest.value?.status], () => { void loadEditorials() })
onMounted(() => { void loadSubmissions(); void loadEditorials() })
usePolling(update, 15000)
// A contest that crosses midnight in Japan shows the date as well, since the time alone could mean either day.
const freezeTime = computed(() => {
  const c = contest.value
  if (!c?.freezeAt) return ''
  const day = (value: string) => new Intl.DateTimeFormat('ja-JP', { dateStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
  const date = day(c.startsAt) !== day(c.endsAt) ? { month: 'long', day: 'numeric' } as const : {}
  return new Intl.DateTimeFormat('ja-JP', { ...date, hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' }).format(new Date(c.freezeAt))
})
// Setters and operators publish the frozen results after the end; the server refuses while runs are still pending.
const revealing = ref(false)
const revealError = ref('')
const standingsTitle = ref<HTMLElement>()
// A refusal no longer applies once someone else has published the results.
watch(() => contest.value?.canRevealResults, can => { if (!can) revealError.value = '' })
async function reveal() {
  if (revealing.value || !window.confirm('結果を公開します。凍結後の提出の結果が順位表に反映され、元に戻せません。よろしいですか？')) return
  revealing.value = true; revealError.value = ''
  try {
    await $fetch(`/api/my/contests/${encodeURIComponent(id)}/reveal`, { method: 'POST' })
    await Promise.all([refresh(), refreshStandings()])
    showToast('結果を公開しました')
    // The focused button is gone with the reveal, so focus moves to the heading of the table it changed.
    await nextTick()
    standingsTitle.value?.focus()
  } catch (error) {
    const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code
    revealError.value = code === 'contest_results_pending' ? '判定待ちの提出が残っているため、まだ結果を公開できません。判定が終わってから、もう一度お試しください。'
      : code === 'contest_conflict' ? '終了前のコンテストと、順位表を凍結しないコンテストでは結果を公開できません。'
      : '結果を公開できませんでした。時間をおいてもう一度お試しください。'
  } finally { revealing.value = false }
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
useSharePreview({ enabled: () => !!contest.value, type: 'website', title: () => contest.value!.title, path: () => `/contests/${shortId(contest.value!.id)}`, description: () => contest.value!.description.slice(0, 160) })
</script>
<template>
  <div v-if="contest" class="problem-page contest-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><template v-if="activeView === 'overview'"><NuxtLink to="/contests">コンテスト</NuxtLink><span aria-hidden="true">/</span><span>{{ contest.title }}</span></template><template v-else><NuxtLink :to="`/contests/${shortId(contest.id)}`">{{ contest.title }}</NuxtLink><span aria-hidden="true">/</span><span>{{ contestViews[activeView] }}</span></template></nav><TweetButton :title="contest.title" :url="`/contests/${shortId(contest.id)}`" /></div>
    <header v-if="activeView === 'overview'" class="problem-header">
      <div class="contest-heading">
        <h1>{{ contest.title }}</h1>
        <NuxtLink v-if="contest.canEdit" class="editor-button contest-edit" :to="`/my/contests/${shortId(contest.id)}`"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg>コンテストを編集</NuxtLink>
        <NuxtLink v-else-if="contest.setter" class="editor-button contest-edit" :to="`/my/contests/${shortId(contest.id)}?section=manage`"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="9" cy="8" r="3" /><path d="M3 20a6 6 0 0 1 12 0M16 4a3 3 0 0 1 0 6M21 20a6 6 0 0 0-4-5.7" /></svg>共同作成者を管理</NuxtLink>
      </div>
      <div class="problem-summary"><div class="problem-meta muted"><span class="contest-status" :data-status="contest.status">{{ contestStatus[contest.status] }}</span><p>作成者 <template v-for="(author, index) in contestAuthors(contest)" :key="author"><span v-if="index">、</span><UserLink :handle="author" /></template></p><p v-if="contest.problemAuthors?.length">作問者 <template v-for="(author, index) in contest.problemAuthors" :key="author"><span v-if="index">、</span><UserLink :handle="author" /></template></p><p v-if="contest.testers?.length">テスター <template v-for="(tester, index) in contest.testers" :key="tester"><span v-if="index">、</span><UserLink :handle="tester" /></template></p></div></div>
      <dl class="limits contest-schedule"><div><dt>開始日時</dt><dd><time :datetime="contest.startsAt">{{ contestDate(contest.startsAt) }}</time></dd></div><div><dt>終了日時</dt><dd><time :datetime="contest.endsAt">{{ contestDate(contest.endsAt) }}</time></dd></div></dl>
      <p class="schedule-timezone muted">日時は日本時間で表示しています。</p>
      <p class="contest-scoring muted">誤答ペナルティ {{ contest.penaltyMinutes }} 分 · 部分点なし</p>
      <p v-if="contest.freezeMinutes" class="contest-scoring muted">終了{{ contest.freezeMinutes }}分前から順位表を凍結します。</p>
    </header>
    <ContestParticipation :contest="contest" @joined="value => { contest = value; refreshStandings() }" />
    <section v-if="activeView === 'overview'" id="overview" class="contest-section contest-description" aria-labelledby="overview-title">
      <h2 id="overview-title">概要</h2><ProblemMarkdown v-if="contest.description" :source="contest.description" /><p v-else class="muted">コンテストの説明はまだありません。</p>
      <p><NuxtLink to="/docs/contest-rules">ルール</NuxtLink></p>
    </section>
    <section v-if="activeView === 'problems'" id="problems" class="contest-section" aria-labelledby="problems-title">
      <h1 id="problems-title">問題</h1>
      <p v-if="contest.status === 'scheduled'" class="notice">問題文は開始時刻に公開されます。</p>
      <div v-if="contest.problems.length" class="content-table-scroll" role="region" aria-label="コンテストの問題" tabindex="0">
        <table class="content-table contest-problems"><thead><tr><th scope="col">#</th><th scope="col">問題</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">配点</th></tr></thead><tbody><tr v-for="(p, index) in contest.problems" :key="p.id || index" :class="{ solved: p.solved }"><td>{{ problemLabel(index) }}</td><th scope="row"><NuxtLink v-if="p.id" :to="contestProblemPath(contest.id, index)" :aria-label="p.solved ? `${p.title}（AC 済み）` : undefined">{{ p.title }}</NuxtLink><span v-else>???</span></th><td class="problem-limits">{{ !p.timeLimitMs ? '—' : `${Number(p.timeLimitMs) / 1000} 秒` }}・{{ !p.memoryLimitMb ? '—' : `${p.memoryLimitMb} MiB` }}</td><td>{{ p.points }} 点</td></tr></tbody></table>
      </div>
    </section>
    <section v-if="activeView === 'standings'" id="standings" class="contest-section" aria-labelledby="standings-title">
      <header class="contest-section-heading"><h1 id="standings-title" ref="standingsTitle" tabindex="-1">公式順位表</h1><div class="standings-actions"><button v-if="contest.canRevealResults" type="button" class="editor-button contest-reveal" :disabled="revealing" :aria-busy="revealing" @click="reveal">{{ revealing ? '公開中…' : '結果を公開' }}</button><RefreshButton label="今すぐ更新" :busy="updating" :disabled="updating" @click="update" /></div></header>
      <p v-if="revealError" class="notice notice-error" role="alert">{{ revealError }}</p>
      <p v-if="user && (contest.excluded || contest.staff)" class="notice">{{ contest.excluded ? 'このコンテストのスタッフを務めたため、提出は公式順位の対象外です。' : 'スタッフ（作成者・作問者・テスター）としての提出は公式順位の対象外です。' }}</p>
      <div v-if="canSwitchStandings" class="standings-mode"><SegmentedChoice v-model="standingsMode" legend="順位表の表示" name="standings-mode" :options="standingsModes" hide-legend /></div>
      <p v-if="showLive" class="notice">凍結なしの順位表を表示しています。スタッフだけが見られます。</p>
      <p v-else-if="frozen" class="notice">{{ freezeTime }}以降の提出の結果は、結果発表まで伏せています。</p>
      <p v-if="standingsError" class="notice notice-error" role="alert">順位表を取得できませんでした。</p>
      <LoadingSpinner v-if="!standingsError && switchingStandings" label="順位表を読み込み中" />
      <p v-else-if="!standingsError && !standings?.length" class="contest-empty muted">参加者はまだいません。</p>
      <div v-if="!standingsError && !switchingStandings && contest.problems.length" class="content-table-scroll" role="region" aria-label="順位表のスクロール領域" tabindex="0" :aria-busy="updating">
        <table class="content-table standings-table">
          <thead><tr><th scope="col">順位</th><th scope="col">ユーザー</th><th scope="col">得点</th><th scope="col">時間</th><th v-for="(p, i) in contest.problems" :key="p.id || i" scope="col"><NuxtLink v-if="contest.status !== 'scheduled' && p.id" :to="contestProblemPath(contest.id, i)">{{ problemLabel(i) }}</NuxtLink><span v-else>{{ problemLabel(i) }}</span></th></tr></thead>
          <tbody>
            <tr v-for="row in standings" :key="row.handle">
              <td>{{ row.rank }}</td><th scope="row"><UserLink :handle="row.handle" /></th><td class="standing-total">{{ row.points }}</td><td>{{ duration(row.timeMs) }}</td>
              <td v-for="(p, i) in contest.problems" :key="p.id || i">
                <template v-if="row.problems[p.id]">
                  <span class="standing-result">
                    <strong v-if="row.problems[p.id]!.acceptedAt" class="standing-accepted">{{ row.problems[p.id]!.points }}</strong>
                    <span v-else-if="!row.problems[p.id]!.wrong && !row.problems[p.id]!.pending && !row.problems[p.id]!.frozen" class="muted">—</span>
                    <span v-if="row.problems[p.id]!.wrong" class="standing-wrong" :aria-label="`誤答 ${row.problems[p.id]!.wrong} 回`">({{ row.problems[p.id]!.wrong }})</span>
                    <svg v-if="row.problems[p.id]!.pending" class="standing-pending" viewBox="0 0 24 24" role="img" aria-label="判定待ち"><path d="M5 3h14M5 21h14M7 3v4l5 5-5 5v4M17 3v4l-5 5 5 5v4" /></svg>
                    <span v-if="row.problems[p.id]!.frozen" class="standing-frozen" :aria-label="`凍結後の提出 ${row.problems[p.id]!.frozen} 件`">?{{ row.problems[p.id]!.frozen }}</span>
                  </span>
                  <small v-if="row.problems[p.id]!.acceptedAt">{{ elapsed(row.problems[p.id]!.acceptedAt!) }}</small>
                </template>
                <span v-else class="muted">—</span>
              </td>
            </tr>
          </tbody>
          <tfoot>
            <tr><th colspan="4" scope="row">FA（初正解）</th><td v-for="(stat, i) in problemStats" :key="stat.id || i"><template v-if="stat.accepted"><span v-for="handle in stat.handles" :key="handle" class="standing-fa"><UserLink :handle="handle" /></span><small>{{ stat.time }}</small></template><span v-else class="muted">—</span></td></tr>
            <tr><th colspan="4" scope="row">正解者数 / 提出者数</th><td v-for="(stat, i) in problemStats" :key="stat.id || i"><span class="standing-accepted">{{ stat.accepted }}</span> / {{ stat.submitted }}</td></tr>
          </tfoot>
        </table>
      </div>
    </section>
    <section v-if="activeView === 'submissions'" id="submissions" class="contest-section" aria-labelledby="submissions-title">
      <h1 id="submissions-title">提出一覧</h1>
      <p v-if="!canViewSubmissions" class="notice">提出一覧と提出コードはコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。</p>
      <template v-else><p class="muted">提出コードを閲覧できます。練習提出も掲載します。</p>
      <p v-if="submissionsError" class="notice notice-error" role="alert">{{ submissionsError }}</p>
      <LoadingSpinner v-if="!submissions && !submissionsError" label="提出一覧を読み込み中" />
      <template v-if="submissions"><p v-if="!submissions.items.length" class="contest-empty muted">提出はまだありません。</p><div v-else class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0"><table class="content-table"><thead><tr><th scope="col">問題</th><th scope="col">ユーザー</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th scope="col">結果</th><th scope="col">提出日時（日本時間）</th></tr></thead><tbody><tr v-for="s in submissions.items" :key="s.id"><th scope="row"><NuxtLink :to="`/contests/${shortId(contest.id)}/submissions/${shortId(s.id)}`">{{ s.problemTitle }}</NuxtLink></th><td><UserLink :handle="s.author" /></td><td class="submission-usage">{{ s.sourceBytes == null ? '—' : `${s.sourceBytes.toLocaleString('en-US')} bytes` }}</td><td class="submission-usage">{{ submissionUsage(s.result) }}</td><td><SubmissionStatus :item="s" /></td><td>{{ contestDate(s.createdAt) }}<span v-if="new Date(s.createdAt) >= new Date(contest.endsAt)"> · 練習</span></td></tr></tbody></table></div><ContentPagination :index="offset / 50" :has-next="submissions.hasMore" :loading="updating" @move="direction => offset += direction * 50" /></template>
      </template>
    </section>
    <section v-if="activeView === 'editorials'" id="editorials" class="contest-section" aria-labelledby="editorials-title">
      <h1 id="editorials-title">解説一覧</h1>
      <p v-if="contest.status !== 'ended'" class="notice">解説はコンテスト終了後に公開されます。終了前は作成者と、各問題の作者・テスターが閲覧できます。</p>
      <p v-if="userEditorialsError" class="notice notice-error" role="alert">{{ userEditorialsError }}</p>
      <LoadingSpinner v-else-if="contest.status === 'ended' && !userEditorials" label="ユーザー解説を読み込み中" />
      <div v-if="contest.problems.length" class="content-table-scroll" role="region" aria-label="解説一覧のスクロール領域" tabindex="0">
        <table class="content-table contest-editorials"><thead><tr><th scope="col">#</th><th scope="col">解説</th><th v-if="contest.status === 'ended'" scope="col">ユーザー解説</th></tr></thead><tbody><tr v-for="(p, index) in contest.problems" :key="p.id || index"><td>{{ problemLabel(index) }}</td><th scope="row"><NuxtLink v-if="p.hasEditorial" :to="{ path: contestProblemPath(contest.id, index), query: { view: 'editorial' } }">{{ p.title }}</NuxtLink><template v-else><span>{{ p.title || '???' }}</span><span class="editorial-state muted">{{ p.hasEditorial === false ? '未登録' : contest.status === 'ended' ? '公開待ち' : '終了後に公開' }}</span></template></th><td v-if="contest.status === 'ended'"><ul v-if="editorialsByProblem[p.id]?.length" class="contest-user-editorials"><li v-for="item in editorialsByProblem[p.id]" :key="item.id"><NuxtLink :to="`/blog/${shortId(item.id)}`">{{ item.title }}</NuxtLink> <span class="muted">{{ item.author }}</span></li></ul><span v-else class="muted">—</span></td></tr></tbody></table>
      </div>
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
.contest-edit, .contest-reveal { flex-shrink: 0; min-height: 44px; padding: 8px 16px; font-weight: 600; }
.standings-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; align-items: center; gap: 8px; }
.contest-status { display: inline-flex; align-items: center; min-height: 28px; padding: 2px 10px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); background: var(--color-surface); white-space: nowrap; }
.contest-status[data-status="running"] { color: var(--color-accent); background: var(--color-accent-soft); border-color: var(--color-accent); }
.contest-schedule { gap: 12px 40px; }
.contest-schedule div { flex-wrap: wrap; gap: 4px 12px; }
.schedule-timezone { margin: 8px 0 12px; }
.contest-scoring { margin: 0; }
.contest-section { padding-block: 32px; }
.contest-section > h2, .contest-section-heading { margin: 0 0 20px; }
/* Views other than the overview drop the contest header, so their section title is the page heading. */
.contest-section h1 { margin: 0 0 20px; font-size: 1.25rem; letter-spacing: normal; }
.contest-description { max-width: 52rem; }
.contest-section-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
.contest-section-heading h1 { margin: 0; }
.contest-problems tr.solved { background: var(--color-accent-soft); }
.contest-problems .problem-limits { white-space: nowrap; font-variant-numeric: tabular-nums; }
.contest-problems td:first-child, .contest-editorials td:first-child { width: 3rem; color: var(--color-muted); }
.editorial-state { margin-left: 12px; font-weight: 400; }
.contest-user-editorials { margin: 0; padding: 0; list-style: none; }
.contest-user-editorials li + li { margin-top: 6px; }
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
.standings-mode { max-width: 24rem; margin-bottom: 16px; }
.standing-frozen { color: var(--color-muted); font-weight: 600; }
.standing-fa { display: block; }
.standings-table small { font-size: .75rem; }
small { display: block; color: var(--color-muted); }
@media (max-width: 39.999rem) { .contest-section-heading { flex-wrap: nowrap; } .contest-section-heading h1 { min-width: 0; } }
</style>
