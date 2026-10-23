<script setup lang="ts">
import { submissionUsage } from '~/utils/submission-usage'
import { formatScore } from '~/utils/score'
import { clampPage, lastPage, pageAfterResize } from '~/utils/pagination'
import type { PageSize } from '~/composables/usePageSize'
import { contestAuthors, contestDate, contestFreezeTime, contestProblemPath, contestStatus, contestView, contestViews, frozenSubmissionsNote, problemLabel, type Standing, type StandingCell } from '~~/shared/types/contest'
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
const liveChosen = ref(false)
const showLive = computed(() => liveChosen.value && canSwitchStandings.value)
// 全員 or オンサイトのみ, kept in the URL and shared by the standings and the submissions (ADR 0024).
const { onsite: onsiteOnly } = useOnsiteScope(() => contest.value?.onsiteDivision === true)
const teamContest = computed(() => contest.value?.teamMode === true)
// The rows carry the view they were read for. While a switch loads, the other view's rows are hidden, so the live table
// never sits under the frozen notice (ADR 0023) nor the frozen one under the live notice, and the whole table never
// passes for the onsite one.
const requestFetch = useRequestFetch()
const { data: standingsData, error: standingsError, refresh: refreshStandings } = await useAsyncData(`contest-standings:${id}`, async (_, { signal }) => {
  const live = showLive.value, onsite = onsiteOnly.value
  const query = { ...live ? { live: '1' } : {}, ...onsite ? { scope: 'onsite' } : {} }
  return { live, onsite, rows: await requestFetch<Standing[]>(`${base}/standings`, { query, signal }) }
}, { watch: [showLive, onsiteOnly] })
const switchingStandings = computed(() => !!standingsData.value && (standingsData.value.live !== showLive.value || standingsData.value.onsite !== onsiteOnly.value))
const standings = computed(() => switchingStandings.value ? undefined : standingsData.value?.rows)
// Team contests mix team rows, which have no handle, and individuals' rows; a team's id cannot collide with a handle.
const standingKey = (row: Standing) => row.team?.id ?? row.handle
// The API returns every row and the table is split here, so the footer still counts them all (ADR 0028). The page holds
// across the refresh, and once rows drop and it is gone the last page shows instead.
const { requested: requestedPage, link: pageLink, go: goToPage } = usePageQuery()
const standingsSize = usePageSize('standings')
const standingsTotal = computed(() => standings.value?.length ?? 0)
const standingsLast = computed(() => lastPage(standingsTotal.value, standingsSize.value))
const standingsPage = computed(() => clampPage(requestedPage.value, standingsLast.value))
// The page's rows are the positions from standingsStart up to, not including, standingsEnd.
const standingsStart = computed(() => (standingsPage.value - 1) * standingsSize.value)
const standingsEnd = computed(() => Math.min(standingsStart.value + standingsSize.value, standingsTotal.value))
const pageStandings = computed(() => standings.value?.slice(standingsStart.value, standingsEnd.value))
// The count and the page numbers come with the table once it has a row.
const showStandingsPages = computed(() => !standingsError.value && standingsTotal.value > 0 && !!contest.value?.problems.length)
function resizeStandings(size: PageSize) {
  const page = pageAfterResize(standingsPage.value, standingsSize.value, size)
  standingsSize.value = size
  void goToPage(page)
}
// Like the onsite switch, choosing the frozen or the live table starts over at the first page.
watch(liveChosen, () => { void goToPage(1) })
const canViewSubmissions = computed(() => contest.value?.canViewSubmissions === true)
// While the contest runs a team member sees only the team's submissions, so the onsite filter has nothing to narrow.
const teamOnly = computed(() => !!contest.value?.team && contest.value.status === 'running')
const filterSubmissions = computed(() => onsiteOnly.value && !teamOnly.value)
const submissions = ref<{ items: Submission[], hasMore: boolean } | null>(null)
const submissionsError = ref('')
const offset = ref(0)
const { run: requestSubmissions, invalidate: invalidateSubmissions } = useLatestRequest()
function loadSubmissions() {
  if (activeView.value !== 'submissions' || !canViewSubmissions.value) { invalidateSubmissions(); submissions.value = null; return Promise.resolve() }
  const query = { offset: offset.value, ...filterSubmissions.value ? { scope: 'onsite' } : {} }
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
watch(filterSubmissions, () => { offset.value = 0; void loadSubmissions() })
watch([activeView, () => contest.value?.status], () => { void loadEditorials() })
onMounted(() => { void loadSubmissions(); void loadEditorials() })
usePolling(update, 15000)
const freezeTime = computed(() => contest.value ? contestFreezeTime(contest.value) : '')
const frozenNote = computed(() => contest.value ? frozenSubmissionsNote(contest.value) : '')
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
// A cell shows the problem's score once it is above 0 (ADR 0025). Rows without score come from servers before
// partial scoring, where only an AC scored.
function cellScore(cell: StandingCell) {
  if (cell.score === undefined) return cell.acceptedAt ? String(cell.points) : ''
  return cell.score > 0 ? cell.scoreText ?? formatScore(cell.score) : ''
}
// The time of the last submission that raised the score; for a full score it is when the score was reached.
function cellTime(cell: StandingCell) {
  return cell.score === undefined ? cell.acceptedAt : cell.score > 0 ? cell.scoredAt ?? cell.acceptedAt : undefined
}
// With partial scores, 「解いた」 means the full score (ADR 0025), so the footer names it.
const solvedWord = computed(() => contest.value?.partialScoring ? '満点' : '正解')
const showSubmissionScore = computed(() => contest.value?.partialScoring === true || !!submissions.value?.items.some(item => item.result?.scoring))
function elapsed(acceptedAt: string) { return duration(new Date(acceptedAt).getTime() - new Date(contest.value!.startsAt).getTime()) }
const problemStats = computed(() => contest.value!.problems.map(problem => {
  let submitted = 0
  let accepted = 0
  let firstAt = Infinity
  // Counted from the rows on screen, so the onsite table names the first onsite solver.
  let first: Standing[] = []
  for (const row of standings.value ?? []) {
    const score = row.problems[problem.id]
    if (!score) continue
    submitted++
    if (!score.acceptedAt) continue
    accepted++
    const at = new Date(score.acceptedAt).getTime()
    if (at < firstAt) { firstAt = at; first = [row] }
    else if (at === firstAt) first.push(row)
  }
  return { id: problem.id, submitted, accepted, first, time: accepted ? duration(firstAt - new Date(contest.value!.startsAt).getTime()) : '' }
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
      <p class="schedule-timezone muted">日時はJSTで表示しています。</p>
      <p class="contest-scoring muted">誤答ペナルティ {{ contest.penaltyMinutes }} 分<template v-if="!contest.partialScoring"> · 部分点なし</template></p>
      <p v-if="contest.freezeMinutes" class="contest-scoring muted">終了{{ contest.freezeMinutes }}分前から順位表を凍結します。</p>
      <p v-if="contest.teamMode" class="contest-scoring muted">チーム戦{{ contest.maxTeamSize ? `（1チーム${contest.maxTeamSize}人まで）` : '' }}</p>
      <p v-if="contest.onsiteDivision" class="contest-scoring muted">オンサイト参加とオープン参加の区分があります。</p>
    </header>
    <ContestParticipation :contest="contest" :detailed="activeView === 'overview'" @joined="value => { contest = value; refreshStandings() }" />
    <section v-if="activeView === 'overview'" id="overview" class="contest-section contest-description" aria-labelledby="overview-title">
      <h2 id="overview-title">概要</h2><ProblemMarkdown v-if="contest.description" :source="contest.description" /><p v-else class="muted">コンテストの説明はまだありません。</p>
      <p><NuxtLink to="/docs/contest-rules">ルール</NuxtLink></p>
    </section>
    <ContestSetterTools v-if="activeView === 'overview' && contest.setter && (contest.teamMode || contest.onsiteDivision)" :contest="contest" @changed="refreshStandings()" />
    <ContestClarifications v-if="activeView === 'clarifications'" :key="user?.id ?? 'anonymous'" :contest="contest" />
    <section v-if="activeView === 'problems'" id="problems" class="contest-section" aria-labelledby="problems-title">
      <h1 id="problems-title">問題</h1>
      <p v-if="contest.status === 'scheduled'" class="notice">問題文は開始時刻に公開されます。</p>
      <div v-if="contest.problems.length" class="content-table-scroll" role="region" aria-label="コンテストの問題" tabindex="0">
        <table class="content-table contest-problems"><thead><tr><th scope="col">#</th><th scope="col">問題</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">配点</th></tr></thead><tbody><tr v-for="(p, index) in contest.problems" :key="p.id || index" :class="{ solved: p.solved }"><td>{{ problemLabel(index) }}</td><th scope="row"><NuxtLink v-if="p.id" :to="contestProblemPath(contest.id, index)" :aria-label="p.solved ? `${p.title}（${p.partial ? '満点' : 'AC 済み'}）` : undefined">{{ p.title }}</NuxtLink><span v-else>???</span></th><td class="problem-limits">{{ !p.timeLimitMs ? '—' : `${Number(p.timeLimitMs) / 1000} 秒` }}・{{ !p.memoryLimitMb ? '—' : `${p.memoryLimitMb} MiB` }}</td><td>{{ p.points }} 点<small v-if="p.partial">部分点あり</small></td></tr></tbody></table>
      </div>
    </section>
    <section v-if="activeView === 'standings'" id="standings" class="contest-section" aria-labelledby="standings-title">
      <header class="contest-section-heading"><h1 id="standings-title" ref="standingsTitle" tabindex="-1">公式順位表</h1><div class="standings-actions"><button v-if="contest.canRevealResults" type="button" class="editor-button contest-reveal" :disabled="revealing" :aria-busy="revealing" @click="reveal">{{ revealing ? '公開中…' : '結果を公開' }}</button><RefreshButton label="今すぐ更新" :busy="updating" :disabled="updating" @click="update" /></div></header>
      <p v-if="revealError" class="notice notice-error" role="alert">{{ revealError }}</p>
      <div v-if="canSwitchStandings || contest.onsiteDivision" class="standings-filters">
        <InlineSwitch v-if="canSwitchStandings" v-model="liveChosen">凍結なしで表示</InlineSwitch>
        <InlineSwitch v-if="contest.onsiteDivision" v-model="onsiteOnly">オンサイトのみ表示</InlineSwitch>
      </div>
      <p v-if="frozen && !showLive" class="notice">{{ freezeTime }}以降の提出の結果は、結果発表まで伏せています。</p>
      <p v-if="onsiteOnly" class="muted standings-scope-note">オンサイト参加{{ teamContest ? 'のチームと個人' : 'の人' }}だけを、振り直した順位で表示しています。小さい数字は全体の順位です。</p>
      <p v-if="standingsError" class="notice notice-error" role="alert">順位表を取得できませんでした。</p>
      <LoadingSpinner v-if="!standingsError && switchingStandings" label="順位表を読み込み中" />
      <p v-else-if="!standingsError && !standings?.length" class="contest-empty muted">{{ onsiteOnly ? 'オンサイトの参加者はまだいません。' : '参加者はまだいません。' }}</p>
      <ListToolbar v-if="showStandingsPages" :total="standingsTotal" :page="standingsPage" :size="standingsSize" unit="件" :link="pageLink" @update:size="resizeStandings" />
      <div v-if="!standingsError && !switchingStandings && contest.problems.length" class="content-table-scroll" role="region" aria-label="順位表のスクロール領域" tabindex="0" :aria-busy="updating">
        <table class="content-table standings-table">
          <thead><tr><th scope="col">順位</th><th scope="col">{{ teamContest ? 'チーム・個人' : 'ユーザー' }}</th><th scope="col">得点</th><th scope="col">時間</th><th v-for="(p, i) in contest.problems" :key="p.id || i" scope="col"><NuxtLink v-if="contest.status !== 'scheduled' && p.id" :to="contestProblemPath(contest.id, i)">{{ problemLabel(i) }}</NuxtLink><span v-else>{{ problemLabel(i) }}</span></th></tr></thead>
          <tbody>
            <tr v-for="row in pageStandings" :key="standingKey(row)">
              <td>{{ row.rank }}<small v-if="onsiteOnly && row.overallRank" class="standing-overall">全体 {{ row.overallRank }} 位</small></td>
              <th scope="row">
                <template v-if="row.team"><span class="standing-team">{{ row.team.name }}</span><span class="standing-members"><template v-for="(member, index) in row.team.members" :key="member"><span v-if="index">、</span><UserLink :handle="member" /></template></span></template>
                <UserLink v-else :handle="row.handle" />
                <span v-if="row.onsite && !onsiteOnly" class="standing-onsite">オンサイト</span>
              </th><td class="standing-total">{{ row.scoreText ?? row.points }}</td><td>{{ duration(row.timeMs) }}</td>
              <td v-for="(p, i) in contest.problems" :key="p.id || i">
                <template v-if="row.problems[p.id]">
                  <span class="standing-result">
                    <strong v-if="cellScore(row.problems[p.id]!)" :class="row.problems[p.id]!.acceptedAt ? 'standing-accepted' : 'standing-partial'">{{ cellScore(row.problems[p.id]!) }}<span v-if="!row.problems[p.id]!.acceptedAt" class="visually-hidden">（部分点）</span></strong>
                    <span v-else-if="!row.problems[p.id]!.wrong && !row.problems[p.id]!.pending && !row.problems[p.id]!.frozen" class="muted">—</span>
                    <span v-if="row.problems[p.id]!.wrong" class="standing-wrong"><span aria-hidden="true">({{ row.problems[p.id]!.wrong }})</span><span class="visually-hidden">誤答 {{ row.problems[p.id]!.wrong }} 回</span></span>
                    <svg v-if="row.problems[p.id]!.pending" class="standing-pending" viewBox="0 0 24 24" role="img" aria-label="判定待ち"><path d="M5 3h14M5 21h14M7 3v4l5 5-5 5v4M17 3v4l-5 5 5 5v4" /></svg>
                    <span v-if="row.problems[p.id]!.frozen" class="standing-frozen"><span aria-hidden="true">?{{ row.problems[p.id]!.frozen }}</span><span class="visually-hidden">凍結後の提出 {{ row.problems[p.id]!.frozen }} 件</span></span>
                  </span>
                  <small v-if="cellTime(row.problems[p.id]!)">{{ elapsed(cellTime(row.problems[p.id]!)!) }}</small>
                </template>
                <span v-else class="muted">—</span>
              </td>
            </tr>
          </tbody>
          <tfoot>
            <tr><th colspan="4" scope="row">{{ contest.partialScoring ? (onsiteOnly ? 'FA（初の満点・オンサイト）' : 'FA（初の満点）') : (onsiteOnly ? 'FA（初正解・オンサイト）' : 'FA（初正解）') }}</th><td v-for="(stat, i) in problemStats" :key="stat.id || i"><template v-if="stat.accepted"><span v-for="row in stat.first" :key="standingKey(row)" class="standing-fa"><template v-if="row.team">{{ row.team.name }}</template><UserLink v-else :handle="row.handle" /></span><small>{{ stat.time }}</small></template><span v-else class="muted">—</span></td></tr>
            <tr><th colspan="4" scope="row">{{ teamContest ? `${solvedWord}数 / 提出数（チーム・個人${onsiteOnly ? '、オンサイト' : ''}）` : `${solvedWord}者数 / 提出者数${onsiteOnly ? '（オンサイト）' : ''}` }}</th><td v-for="(stat, i) in problemStats" :key="stat.id || i"><span class="standing-accepted">{{ stat.accepted }}</span> / {{ stat.submitted }}</td></tr>
          </tfoot>
        </table>
      </div>
      <PageNav v-if="showStandingsPages" class="standings-pages" :page="standingsPage" :last="standingsLast" :link="pageLink" />
    </section>
    <section v-if="activeView === 'submissions'" id="submissions" class="contest-section" aria-labelledby="submissions-title">
      <h1 id="submissions-title">提出一覧</h1>
      <p v-if="!canViewSubmissions" class="notice">提出一覧と提出コードはコンテスト終了後に公開されます。終了前はスタッフ（作成者・作問者・テスター）が閲覧できます。<template v-if="contest.teamMode">チームのメンバーは、開催中もチームの提出を閲覧できます。</template>{{ frozenNote }}</p>
      <template v-else><p class="muted">{{ teamOnly ? '開催中は、チームの提出だけを表示しています。提出コードを閲覧できます。' : '提出コードを閲覧できます。練習提出も掲載します。' }}{{ frozenNote }}</p>
      <div v-if="contest.onsiteDivision && !teamOnly" class="submissions-scope"><InlineSwitch v-model="onsiteOnly">オンサイトのみ表示</InlineSwitch></div>
      <p v-if="submissionsError" class="notice notice-error" role="alert">{{ submissionsError }}</p>
      <LoadingSpinner v-if="!submissions && !submissionsError" label="提出一覧を読み込み中" />
      <template v-if="submissions"><p v-if="!submissions.items.length" class="contest-empty muted">提出はまだありません。</p><div v-else class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0"><table class="content-table"><thead><tr><th scope="col">問題</th><th scope="col">ユーザー</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th v-if="showSubmissionScore" scope="col">得点</th><th scope="col">結果</th><th scope="col">提出日時 (JST)</th></tr></thead><tbody><tr v-for="s in submissions.items" :key="s.id"><th scope="row"><NuxtLink :to="`/contests/${shortId(contest.id)}/submissions/${shortId(s.id)}`">{{ s.problemTitle }}</NuxtLink></th><td><UserLink :handle="s.author" /></td><td class="submission-usage">{{ s.sourceBytes == null ? '—' : `${s.sourceBytes.toLocaleString('en-US')} bytes` }}</td><td class="submission-usage">{{ submissionUsage(s.result) }}</td><td v-if="showSubmissionScore" class="submission-score" :data-full="(s.result?.scoring && s.result.scoring.score === s.result.scoring.maxScore) || undefined">{{ s.result?.scoring ? formatScore(s.result.scoring.score) : '—' }}</td><td><SubmissionStatus :item="s" /></td><td>{{ contestDate(s.createdAt) }}<span v-if="new Date(s.createdAt) >= new Date(contest.endsAt)"> · 練習</span></td></tr></tbody></table></div><ContentPagination :index="offset / 50" :has-next="submissions.hasMore" :loading="updating" @move="direction => offset += direction * 50" /></template>
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
.submission-score { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-score[data-full] { color: var(--color-accent); font-weight: 600; }
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
/* The spoken counts are absolutely positioned; anchoring them here keeps them inside the scrolling table. */
.standing-result { position: relative; display: inline-flex; align-items: center; justify-content: center; gap: 4px; }
.standing-pending { width: 16px; height: 16px; flex-shrink: 0; fill: none; stroke: currentColor; stroke-width: 1.5; stroke-linecap: round; stroke-linejoin: round; color: var(--color-muted); }
.standing-accepted, .standing-fa { color: var(--color-accent); }
/* A partial score keeps the ordinary colour; the accent marks the full score (ADR 0025). */
.standing-partial { color: var(--color-ink); }
.standing-wrong { color: var(--color-error); }
.standings-filters { display: flex; flex-wrap: wrap; gap: 0 24px; margin-bottom: 8px; }
.standings-pages { margin-top: 16px; }
.submissions-scope { margin-top: 8px; }
.standings-scope-note { margin: 0 0 16px; font-size: .8125rem; }
.standing-team { display: block; font-weight: 600; overflow-wrap: anywhere; }
.standing-members { display: block; color: var(--color-muted); font-size: .75rem; font-weight: 400; }
.standing-onsite { display: inline-block; margin-top: 4px; padding: 0 6px; border: 1px solid var(--color-accent); border-radius: 4px; color: var(--color-accent); background: var(--color-accent-soft); font-size: .6875rem; font-weight: 600; white-space: nowrap; }
.standings-table tbody th .standing-onsite { margin-inline-start: 6px; }
.standings-table tbody th:has(.standing-team) .standing-onsite { margin-inline-start: 0; }
.standing-frozen { color: var(--color-muted); font-weight: 600; }
.standing-fa { display: block; }
.standings-table small { font-size: .75rem; }
small { display: block; color: var(--color-muted); }
@media (max-width: 39.999rem) { .contest-section-heading { flex-wrap: nowrap; } .contest-section-heading h1 { min-width: 0; } }
</style>
