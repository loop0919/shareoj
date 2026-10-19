<script setup lang="ts">
import { accountListSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { contestAuthors, problemLabel, type Contest, type ContestDraft, type ContestDraftResult, type DraftProblem, type Venue } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { accountError } from '~/utils/account-problems'
import { issueMessage } from '~/utils/problem-readiness'
import { searchProblems } from '~/utils/problem-search'
import { venuePassphraseRule, venuePassphraseValid } from '~/utils/venue-passphrase'
const props = defineProps<{ contestId?: string }>()
const route = useRoute()
const router = useRouter()
const { showToast } = useToast()
const { profile } = useAccount()
const ready = ref(false)
const busy = ref(false)
const message = ref('')
const title = ref('')
const description = ref('<!-- ここにコンテストの概要を記載 -->\n')
const startsAt = ref('')
// Number inputs hold '' while empty; drafts save that as null.
const durationMinutes = ref<number | string>(120)
const penaltyMinutes = ref<number | string>(5)
// The length of the standings freeze (ADR 0023), used only while the switch is on.
const freezeMinutes = ref<number | string>('')
// The switch is off by default; turning it on fills in a starting value so that on never means no freeze.
const freezeOn = ref(false)
watch(freezeOn, (on) => {
  if (!on) freezeMinutes.value = ''
  else if (freezeMinutes.value === '') freezeMinutes.value = Math.min(60, Number(durationMinutes.value) || 60)
})
// Team contests and the onsite division (ADR 0024). Each optional limit is a switch that shows its input only when on,
// like the freeze.
const teamMode = ref<'individual' | 'team'>('individual')
const teamModes = [{ value: 'individual' as const, label: '個人戦' }, { value: 'team' as const, label: 'チーム戦' }]
const teamSizeOn = ref(false)
const maxTeamSize = ref<number | string>('')
watch(teamSizeOn, (on) => {
  if (!on) maxTeamSize.value = ''
  else if (maxTeamSize.value === '') maxTeamSize.value = 3
})
const onsiteOn = ref(false)
// Setters only: drafts return it as typed, and a published contest reads it from the setters' venue route.
const passphrase = ref('')
// Turning the switch on fills in a passphrase easy to read out and to type on a phone: lower case, without the
// characters join codes leave out, so it always meets the rule. Loading a contest sets both refs directly, so nothing
// is filled in then.
const passphraseAlphabet = 'abcdefghjkmnpqrstvwxyz23456789'
function setOnsite(on: boolean) {
  onsiteOn.value = on
  if (!on) passphrase.value = ''
  else if (!passphrase.value) passphrase.value = Array.from(crypto.getRandomValues(new Uint8Array(8)), byte => passphraseAlphabet[byte % passphraseAlphabet.length]).join('')
}
const maxTeamMembers = 50
const version = ref(0)
const available = ref<AccountSummary[]>([])
const titles = new Map<string, string>()
// Problems the setter joined as a tester rather than wrote.
const tested = reactive(new Set<string>())
// Problems another current setter authors or tests. Problem routes show a setter only their own, so the draft names
// these with their contest readiness (ADR 0022).
const shared = reactive(new Map<string, DraftProblem>())
// Authors of the problems a published contest brings to this setter (viaContest), so they are tagged as in the draft.
const contestAuthorOf = reactive(new Map<string, string>())
// Published problems that no current setter authors or tests; saving fails until they are removed.
const unmanaged = reactive(new Set<string>())
// Setters, owner first. A contest not saved yet has only its creator, who owns it.
const authors = ref<string[]>([])
const isOwner = ref(true)
// After the start a setter can still manage co-setters, though every setting is locked.
const settersOnly = ref(false)
// Once the results are final (at the end, or at the reveal of a frozen contest), invitations close; removing and
// leaving still work.
const resultsFinal = ref(false)
type EditorProblem = { id: string, title?: string, points: number | string | null }
const selected = ref<EditorProblem[]>([])
const id = ref(props.contestId ?? '')
const locked = ref(false)
// Until publication the contest is a draft: saving accepts anything, and the server lists what publishing still needs.
const published = ref(false)
const draftIssues = ref<string[]>([])
const showIssues = ref(false)
const timezone = ref('')
type Section = 'description' | 'problems' | 'schedule' | 'participation' | 'manage'
// The co-setter notification links straight to the management section.
const section = ref<Section>(route.query.section === 'manage' ? 'manage' : 'description')
const sidebarExpanded = ref(false)
const form = ref<HTMLFormElement>()
const totalPoints = computed(() => selected.value.reduce((total, p) => total + (Number(p.points) || 0), 0))
const query = ref('')
const onlyAddable = ref(true)
const summaries = computed(() => new Map(available.value.map(p => [p.id, p])))
const isSelected = (problemId: string) => selected.value.some(item => item.id === problemId)
const addable = (problemId: string) => !isSelected(problemId) && !issues(problemId).length
const matches = computed(() => searchProblems(available.value, query.value))
const candidates = computed(() => onlyAddable.value ? matches.value.filter(p => addable(p.id)) : matches.value)
const problemName = (title?: string) => title || '無題の問題'
const maxDurationMinutes = 100_000_000
const endsAt = computed(() => {
  const start = new Date(startsAt.value), minutes = Number(durationMinutes.value)
  return Number.isFinite(start.getTime()) && Number.isInteger(minutes) && minutes >= 1 && minutes <= maxDurationMinutes ? new Date(start.getTime() + minutes * 60000) : undefined
})
function durationText(minutes: number) {
  const hours = Math.floor(minutes / 60), rest = minutes % 60
  return hours && rest ? `${hours}時間${rest}分` : hours ? `${hours}時間` : `${rest}分`
}
const endLabel = computed(() => endsAt.value
  ? `終了日時 ${new Intl.DateTimeFormat('ja-JP', { dateStyle: 'medium', timeStyle: 'short' }).format(endsAt.value)}（${durationText(Number(durationMinutes.value))}）`
  : '開始日時とコンテスト時間を入力すると、終了日時を表示します。')
const issueGuide: Record<string, [string, Section]> = {
  title_missing: ['コンテストタイトルを入力してください', 'description'],
  start_missing: ['開始日時を入力してください', 'schedule'],
  start_past: ['開始日時を現在より後にしてください', 'schedule'],
  duration_invalid: ['コンテスト時間を1分以上の整数で入力してください', 'schedule'],
  penalty_invalid: ['誤答ペナルティを0〜1440分の整数で入力してください', 'schedule'],
  freeze_invalid: ['順位表を凍結する時間は、コンテスト時間以内の1分以上の整数で入力するか、凍結をオフにしてください', 'schedule'],
  team_size_invalid: ['チームの人数の上限は2〜50人の整数で入力するか、上限をオフにしてください', 'participation'],
  venue_passphrase_invalid: ['会場の合言葉は英数字とアンダースコア（_）の4〜64文字で入力するか、オンサイト区分をオフにしてください', 'participation'],
  problems_missing: ['問題を1問以上選んでください', 'problems'],
  points_invalid: ['配点を1〜1,000,000点の整数で入力してください', 'problems'],
  problem_unavailable: ['公開済み・削除済み・他のコンテストに登録済みの問題と、作成者の誰も作者・テスターではない問題を外してください', 'problems'],
  problem_incomplete: ['難易度やテストケースなど、コンテストの条件を満たしていない問題があります', 'problems'],
}
const issueText = (code: string) => issueGuide[code]?.[0] ?? `確認が必要な項目があります（${code}）`
const sectionNames: Record<Section, string> = { description: 'タイトル・説明', problems: '問題・配点', schedule: '期間・ペナルティ', participation: '参加形式', manage: 'コンテスト管理' }
const issueSection = (code: string): Section => issueGuide[code]?.[1] ?? 'manage'
const snapshot = () => JSON.stringify([title.value, description.value, startsAt.value, durationMinutes.value, penaltyMinutes.value, freezeOn.value, freezeMinutes.value, teamMode.value, teamSizeOn.value, maxTeamSize.value, onsiteOn.value, passphrase.value, selected.value])
const saved = ref('')
let leaving = false
const dirty = computed(() => ready.value && snapshot() !== saved.value)
const status = computed(() => {
  if (busy.value) return '保存中…'
  if (locked.value) return '編集できません'
  if (!ready.value) return ''
  const summary = `${selected.value.length} 問 · 合計 ${totalPoints.value} 点`
  if (published.value) return summary
  const state = dirty.value ? '未保存の変更があります' : !version.value ? 'まだ保存していません' : draftIssues.value.length ? `公開まであと ${draftIssues.value.length} 項目` : '公開できます'
  return `下書き · ${state} · ${summary}`
})
onBeforeRouteLeave(() => leaving || (!busy.value && (!dirty.value || locked.value || window.confirm('未保存の変更を破棄して移動しますか？'))))
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value && !locked.value) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
const {
  mode, workspace, splitPercent, resizing, setSplit, startResize, moveResize, stopResize, resizeWithKeyboard,
  editor, syncSource, insertSnippet, preview, editorScrolled, previewScrolled,
} = useMarkdownEditor()
const mathSnippet = '\n$$\na^2 + b^2 = c^2\n$$\n'
function saveWithShortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    if (ready.value && !locked.value && !busy.value) form.value?.requestSubmit()
  }
}
onMounted(() => window.addEventListener('keydown', saveWithShortcut))
onBeforeUnmount(() => window.removeEventListener('keydown', saveWithShortcut))
function localDate(value: string) {
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}
async function load() {
  try {
    // A new contest keeps its id in the query after the first save, like new problems and posts.
    const resume = props.contestId ?? (typeof route.query.contest === 'string' && /^[a-f0-9-]{36}$/.test(route.query.contest) ? route.query.contest : '')
    id.value = resume || crypto.randomUUID()
    // Own problems first, then those the setter tests: both can be entered.
    for (const role of ['author', 'tester'] as const) {
      let cursor = ''
      do {
        const page = accountListSchema.parse(await $fetch('/api/my/problems', { query: { cursor, ...(role === 'tester' ? { role } : {}) } }))
        for (const p of page.items) {
          titles.set(p.id, p.title)
          // Problems managed only through this contest are not entered by this setter, so they are not marked.
          if (role === 'tester' && !p.viaContest) tested.add(p.id)
          if (p.viaContest?.id === id.value && p.viaContest.author) contestAuthorOf.set(p.id, p.viaContest.author)
        }
        available.value.push(...page.items.filter(p => !p.publishedVersion && (!p.contestId || p.contestId === id.value)))
        cursor = page.nextCursor
      } while (cursor)
    }
    if (resume) await open(resume)
    if (locked.value) return
    saved.value = snapshot()
    ready.value = true
  } catch { message.value = '作成情報を読み込めませんでした。ログイン状態を確認して再読み込みしてください。' }
}
// A draft and a published contest share the id, so try the draft first.
async function open(contestId: string) {
  try {
    const result = await $fetch<ContestDraftResult>(`/api/my/contests/${contestId}/draft`)
    const d = result.draft
    version.value = result.version; draftIssues.value = result.issues; applySetters(result)
    title.value = d.title; description.value = d.description; startsAt.value = d.startsAt ? localDate(d.startsAt) : ''
    durationMinutes.value = d.durationMinutes ?? ''; penaltyMinutes.value = d.penaltyMinutes ?? ''; freezeMinutes.value = d.freezeMinutes ?? ''; freezeOn.value = d.freezeMinutes != null
    applyParticipation(d.teamMode, d.maxTeamSize, d.onsiteDivision, d.onsitePassphrase)
    selected.value = d.problems.map(p => ({ id: p.id, title: titles.get(p.id) ?? shared.get(p.id)?.title ?? '表示できない問題', points: p.points }))
    return
  } catch (error) {
    if ((error as { statusCode?: number }).statusCode !== 404) throw error
  }
  const c = await $fetch<Contest>(`/api/my/contests/${contestId}`)
  published.value = true
  if (!c.canEdit && !c.setter) { locked.value = true; message.value = '開始後、または作成者以外は編集できません。'; return }
  applySetters(c)
  title.value = c.title; description.value = c.description; startsAt.value = localDate(c.startsAt)
  durationMinutes.value = Math.round((Date.parse(c.endsAt) - Date.parse(c.startsAt)) / 60000)
  penaltyMinutes.value = c.penaltyMinutes; freezeMinutes.value = c.freezeMinutes ?? ''; freezeOn.value = c.freezeMinutes != null; version.value = c.version
  // Saving with the division on sends the passphrase back, so an editable contest reads it first.
  const venue = c.canEdit && c.onsiteDivision ? await $fetch<Venue>(`/api/my/contests/${contestId}/onsite`) : null
  applyParticipation(c.teamMode, c.maxTeamSize, c.onsiteDivision, venue?.passphrase)
  selected.value = c.problems.map(p => ({ id: p.id, title: p.title, points: p.points }))
  if (!c.canEdit) { locked.value = true; settersOnly.value = true; resultsFinal.value = c.resultsFinal ?? c.status === 'ended'; section.value = 'manage' }
}
function applyParticipation(team?: boolean, size?: number | null, onsite?: boolean, venue?: string | null) {
  teamMode.value = team ? 'team' : 'individual'
  maxTeamSize.value = size ?? ''; teamSizeOn.value = size != null
  passphrase.value = venue ?? ''; onsiteOn.value = !!onsite
}
// Who sets the contest, and which problems other setters bring; a draft and a contest describe them differently.
function applySetters(result: ContestDraftResult | Contest) {
  if ('draft' in result) {
    authors.value = result.authors ?? []
    shared.clear()
    for (const p of result.problems ?? []) shared.set(p.id, p)
  } else {
    authors.value = contestAuthors(result)
    unmanaged.clear()
    for (const p of result.problems) if (p.unmanaged) unmanaged.add(p.id)
  }
  isOwner.value = result.owner !== false
}
onMounted(() => { timezone.value = Intl.DateTimeFormat().resolvedOptions().timeZone; void load() })
// Why the server would reject this problem; candidates already exclude other contests, so in_contest means this one.
function issues(problemId: string) {
  return (summaries.value.get(problemId)?.readiness?.contest ?? []).filter(code => code !== 'in_contest')
}
// A selected problem missing from the candidates was published, deleted or registered elsewhere after the draft was
// saved, or belongs to another setter, whose problems the saved draft describes.
function problemIssue(problemId: string) {
  // After the start the problem set is fixed, and after the end its problems are published, so nothing applies.
  if (settersOnly.value) return ''
  if (unmanaged.has(problemId)) return '作成者の誰も作者・テスターではないため、外すまで保存できません'
  if (!summaries.value.has(problemId)) {
    const other = shared.get(problemId)
    if (!other) return published.value ? '公開済み・削除済み・他のコンテストに登録済みのため出題できません' : '削除されたか、作成者の誰も作者・テスターではないため出題できません'
    const code = other.issues[0]
    return code && issueMessage(code, 'contest')
  }
  const code = issues(problemId)[0]
  return code && issueMessage(code, 'contest')
}
// Another setter's problem names its author, as the candidates mark the ones this setter tests.
function problemOwner(problemId: string) {
  const author = contestAuthorOf.get(problemId)
  if (author) return author === me.value ? '' : author
  return summaries.value.has(problemId) ? '' : shared.get(problemId)?.author ?? ''
}
function add(p: AccountSummary) {
  if (addable(p.id)) selected.value.push({ id: p.id, title: p.title, points: 100 })
}
// Enter adds the best match, so a pasted UUID needs no pointer; it must not submit the form.
function addFirstCandidate() {
  const p = query.value.trim() ? candidates.value.find(c => addable(c.id)) : undefined
  if (p) { add(p); query.value = '' }
}
function remove(index: number) { selected.value.splice(index, 1) }
function move(from: number, to: number) {
  const item = selected.value.splice(from, 1)[0]
  if (!item) return
  selected.value.splice(to, 0, item)
  reorderStatus.value = `${problemName(item.title)}を${problemLabel(to)}に移動しました`
}
// Rows keep their layout while dragging: the others slide by the dragged row's height, and the order changes on drop.
const list = ref<HTMLOListElement>()
const reorderStatus = ref('')
const drag = ref<{ from: number, to: number, startY: number, y: number, clientY: number, height: number, mids: number[] } | null>(null)
let scrollFrame = 0
const listY = (clientY: number) => clientY - list.value!.getBoundingClientRect().top
function startDrag(event: PointerEvent, index: number) {
  const handle = event.currentTarget as HTMLElement
  if (event.button !== 0 || handle.matches(':disabled') || !list.value) return
  event.preventDefault()
  handle.setPointerCapture(event.pointerId)
  const rows = [...list.value.children] as HTMLElement[], y = listY(event.clientY)
  drag.value = { from: index, to: index, startY: y, y, clientY: event.clientY, height: rows[index]!.offsetHeight, mids: rows.map(row => row.offsetTop + row.offsetHeight / 2) }
  scrollFrame = requestAnimationFrame(autoScroll)
}
function updateDrag(clientY: number) {
  const d = drag.value
  if (!d) return
  d.clientY = clientY
  d.y = listY(clientY)
  const center = d.mids[d.from]! + d.y - d.startY
  d.to = d.mids.filter((mid, i) => i !== d.from && mid < center).length
}
function endDrag(commit: boolean) {
  const d = drag.value
  if (!d) return
  cancelAnimationFrame(scrollFrame)
  drag.value = null
  if (commit && d.to !== d.from) move(d.from, d.to)
}
// Holding a row near the pane's edge scrolls it, so a long list can be reordered in one drag.
function autoScroll() {
  const d = drag.value, pane = list.value?.closest<HTMLElement>('.problem-management')
  if (!d || !pane) return
  const box = pane.getBoundingClientRect(), edge = 48
  const over = d.clientY < box.top + edge ? d.clientY - box.top - edge : d.clientY > box.bottom - edge ? d.clientY - box.bottom + edge : 0
  if (over) { pane.scrollTop += over / 4; updateDrag(d.clientY) }
  scrollFrame = requestAnimationFrame(autoScroll)
}
// Where the row lands if dropped now, so the letters already show the new order.
function slot(index: number) {
  const d = drag.value
  if (!d) return index
  return index === d.from ? d.to : d.from < index && index <= d.to ? index - 1 : d.to <= index && index < d.from ? index + 1 : index
}
function rowStyle(index: number) {
  const d = drag.value
  if (!d) return undefined
  const shift = index === d.from ? d.y - d.startY : (slot(index) - index) * d.height
  return shift ? { transform: `translateY(${shift}px)` } : undefined
}
async function moveWithKeyboard(event: KeyboardEvent, index: number) {
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  const to = index + (event.key === 'ArrowUp' ? -1 : 1)
  if (drag.value || to < 0 || to >= selected.value.length) return
  const id = selected.value[index]!.id
  move(index, to)
  await nextTick()
  list.value?.querySelector<HTMLElement>(`[data-id="${id}"] .drag-handle`)?.focus()
}
onBeforeUnmount(() => cancelAnimationFrame(scrollFrame))
// Jumps to the first field the browser rejects. Only publication and published contests need valid fields.
async function reportInvalid() {
  const invalid = form.value?.querySelector<HTMLInputElement | HTMLTextAreaElement>('input:invalid, textarea:invalid')
  if (!invalid) return true
  section.value = invalid.closest<HTMLElement>('[data-section]')!.dataset.section as Section
  if (section.value === 'description') mode.value = 'edit'
  await nextTick()
  invalid.reportValidity()
  return false
}
const integer = (value: unknown) => typeof value === 'number' && Number.isInteger(value) && Math.abs(value) <= 1e9 ? value : null
function draftBody(): ContestDraft {
  const start = new Date(startsAt.value)
  return {
    title: title.value, description: description.value, startsAt: Number.isFinite(start.getTime()) ? start.toISOString() : null,
    // With the switch on, a length that is not a whole number goes as 0, so the draft keeps the freeze and lists the issue.
    durationMinutes: integer(durationMinutes.value), penaltyMinutes: integer(penaltyMinutes.value), freezeMinutes: freezeOn.value ? integer(freezeMinutes.value) ?? 0 : null,
    // As with the freeze, a switch left on without a whole number keeps the setting and lists the issue.
    ...participationBody(teamSizeOn.value ? integer(maxTeamSize.value) ?? 0 : null),
    problems: selected.value.map(p => ({ id: p.id, points: integer(p.points) })),
  }
}
// Every save sends all four, since the API keeps the stored value of a field left out.
function participationBody(size: number | null) {
  const team = teamMode.value === 'team'
  return { teamMode: team, maxTeamSize: team ? size : null, onsiteDivision: onsiteOn.value, onsitePassphrase: onsiteOn.value ? passphrase.value : null }
}
// A saved draft or a contest answers 404 once the viewer no longer sets it, so retrying cannot help.
const noLongerSetter = 'このコンテストの作成者ではなくなったため保存できません。入力内容をコピーしてください。'
function failure(error: unknown) {
  if ((error as { data?: { data?: { code?: string } } }).data?.data?.code === 'creation_quota_exceeded') return accountError(error)
  const status = (error as { statusCode?: number }).statusCode
  return status === 409 ? '別の画面で更新されたか、すでに公開されています。入力内容をコピーしてから再読み込みしてください。' : status === 404 && version.value ? noLongerSetter : status === 401 ? 'ログインし直してから保存してください。' : status === 400 ? '入力内容を確認してください。' : '保存を確認できませんでした。入力内容を残したまま、もう一度お試しください。'
}
async function saveDraft() {
  if (busy.value || !ready.value || locked.value) return false
  busy.value = true; message.value = ''
  try {
    const current = snapshot()
    const result = await $fetch<ContestDraftResult>(`/api/my/contests/${id.value}/draft`, { method: 'PUT', body: { version: version.value, draft: draftBody() } })
    version.value = result.version; draftIssues.value = result.issues; saved.value = current; applySetters(result)
    if (!props.contestId && route.query.contest !== id.value) await router.replace({ path: '/my/contests/new', query: { contest: id.value } })
    return true
  } catch (error) { message.value = failure(error); return false }
  finally { busy.value = false }
}
async function publish() {
  if (busy.value || !ready.value || locked.value || !await reportInvalid() || !await saveDraft()) return
  // The management section lists every missing item next to a button that opens its section.
  if (draftIssues.value.length) { showIssues.value = true; section.value = 'manage'; return }
  if (!window.confirm('コンテストを公開しますか？コンテスト一覧に表示され、参加登録を受け付けます。開始前なら内容を変更できます。')) return
  busy.value = true
  try {
    await $fetch(`/api/my/contests/${id.value}/publication`, { method: 'PUT', body: { version: version.value } })
    leaving = true
    await navigateTo(`/contests/${shortId(id.value)}`)
    showToast('公開しました')
  } catch (error) {
    const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code
    message.value = code === 'contest_not_ready' ? '公開の条件を満たしていません。再読み込みして内容を確認してください。' : (error as { statusCode?: number }).statusCode === 409 ? '公開できませんでした。問題が別のコンテストに登録されたか、別の画面で更新されています。再読み込みしてください。' : failure(error)
  } finally { busy.value = false }
}
async function removeDraft() {
  if (!version.value || !window.confirm('このコンテストの下書きを削除します。この操作は取り消せません。削除しますか？')) return
  busy.value = true
  try {
    await $fetch(`/api/my/contests/${id.value}/draft`, { method: 'DELETE', query: { version: version.value } })
    leaving = true; busy.value = false
    await navigateTo('/my?tab=contests')
  } catch (error) { message.value = failure(error) }
  finally { busy.value = false }
}
const me = computed(() => profile.value?.handle ?? '')
const setterList = computed(() => authors.value.length ? authors.value : me.value ? [me.value] : [])
const setterLink = ref('')
// The running setter action, so only link issuance reads 「発行中…」.
const setterAction = ref<'' | 'link' | 'remove' | 'leave'>('')
const setterBusy = computed(() => !!setterAction.value)
const setterListElement = ref<HTMLUListElement>()
const setterMessage = ref('')
const setterLinkCopied = ref(false)
let setterCopiedTimer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(setterCopiedTimer))
const canManageSetters = computed(() => (ready.value || settersOnly.value) && !busy.value && !setterBusy.value)
function setterError(error: unknown) {
  if ((error as { data?: { data?: { code?: string } } }).data?.data?.code === 'contest_invitation_closed') return '結果が確定したため、共同作成者の招待はできません。'
  return (error as { statusCode?: number }).statusCode === 404 ? 'このコンテストの作成者ではなくなったか、相手がすでに抜けています。ページを再読み込みしてください。' : accountError(error)
}
// Removal or leaving can leave problems that no setter manages; reread who sets the contest without touching edits.
async function refreshSetters() {
  try {
    if (published.value) applySetters(await $fetch<Contest>(`/api/my/contests/${id.value}`))
    else {
      // The saved draft's issues change too, for example when the removed setter's problem stays listed.
      const result = await $fetch<ContestDraftResult>(`/api/my/contests/${id.value}/draft`)
      draftIssues.value = result.issues; applySetters(result)
    }
  } catch { /* The list already reflects the change; the next save rereads the rest. */ }
}
async function setterInvitation(renew: boolean) {
  if (!canManageSetters.value || (renew && !window.confirm('招待リンクを作り直しますか？今のリンクは使えなくなります。'))) return
  setterAction.value = 'link'; setterMessage.value = ''
  try {
    // The link belongs to the saved draft, so a contest that was never saved is saved first.
    if (!published.value && !version.value && !await saveDraft()) return
    const result = await $fetch<{ token: string }>(`/api/my/contests/${id.value}/setter-invitation`, { method: renew ? 'PUT' : 'POST' })
    setterLink.value = new URL(`/my/contest-invitations/${result.token}`, window.location.origin).href
    if (renew) setterMessage.value = '招待リンクを作り直しました。前のリンクは使えません。'
  } catch (error) { setterMessage.value = setterError(error) }
  finally { setterAction.value = '' }
}
async function copySetterLink() {
  try {
    await navigator.clipboard.writeText(setterLink.value)
    setterMessage.value = ''
    setterLinkCopied.value = true
    clearTimeout(setterCopiedTimer)
    setterCopiedTimer = setTimeout(() => { setterLinkCopied.value = false }, 2000)
  } catch { setterMessage.value = 'リンクを選択してコピーしてください。' }
}
async function removeSetter(handle: string) {
  if (!canManageSetters.value || !window.confirm(`${handle}を共同作成者から外しますか？外した人は、このコンテストに参加できなくなります。招待リンクは作り直され、今のリンクは使えなくなります。`)) return
  setterAction.value = 'remove'; setterMessage.value = ''
  try {
    await $fetch(`/api/my/contests/${id.value}/setters/${encodeURIComponent(handle)}`, { method: 'DELETE' })
    authors.value = authors.value.filter(author => author !== handle)
    // Removal renews the link, so the one on screen no longer works.
    setterLink.value = ''
    setterMessage.value = `${handle}を共同作成者から外しました。招待リンクは作り直されました。`
    // The focused 「外す」 button is gone with its row, so focus stays in the list instead of falling to the page.
    await nextTick()
    setterListElement.value?.focus()
    await refreshSetters()
  } catch (error) { setterMessage.value = setterError(error) }
  finally { setterAction.value = '' }
}
async function leaveContest() {
  if (!canManageSetters.value || !window.confirm('このコンテストの共同作成者から抜けますか？抜けると編集できなくなり、このコンテストに参加することもできません。保存していない変更は失われます。')) return
  setterAction.value = 'leave'; setterMessage.value = ''
  try {
    await $fetch(`/api/my/contests/${id.value}/setters/${encodeURIComponent(me.value)}`, { method: 'DELETE' })
    leaving = true
    await navigateTo('/my?tab=contests')
    showToast('共同作成者から抜けました')
  } catch (error) { setterMessage.value = setterError(error) }
  finally { setterAction.value = '' }
}
async function save() {
  if (!published.value) return saveDraft()
  if (busy.value || !ready.value || locked.value) return
  message.value = ''
  if (!await reportInvalid()) return
  if (!title.value.trim()) { section.value = 'description'; message.value = 'コンテストタイトルを入力してください。'; return }
  if (!selected.value.length || selected.value.length > 100) {
    section.value = 'problems'; message.value = '問題を1〜100問選んでください。'; return
  }
  const start = new Date(startsAt.value), end = endsAt.value
  if (!Number.isFinite(start.getTime()) || start.getTime() <= Date.now() || !end) {
    section.value = 'schedule'; message.value = '未来の開始日時と、1分以上のコンテスト時間を指定してください。'; return
  }
  // The server checks the passphrase after NFKC and trimming; the field's own check covers what the browser can see.
  if (onsiteOn.value && !venuePassphraseValid(passphrase.value)) {
    section.value = 'participation'; message.value = `${issueText('venue_passphrase_invalid')}。`; return
  }
  busy.value = true; message.value = ''
  try {
    await $fetch(`/api/my/contests/${id.value}`, { method: 'PUT', body: { title: title.value, description: description.value, startsAt: start.toISOString(), endsAt: end.toISOString(), penaltyMinutes: penaltyMinutes.value, freezeMinutes: freezeOn.value ? freezeMinutes.value : null, ...participationBody(teamSizeOn.value ? Number(maxTeamSize.value) : null), version: version.value, problems: selected.value.map(({ id, points }) => ({ id, points })) } })
    leaving = true
    await navigateTo(`/contests/${shortId(id.value)}`)
  } catch (error) {
    const status = (error as { statusCode?: number }).statusCode
    const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code
    if (code === 'contest_participant_problem') {
      section.value = 'problems'; message.value = '参加登録した人が作者・テスターである問題は出題できません。該当する問題を外してください。'; return
    }
    if (code === 'contest_team_mode_locked') { section.value = 'participation'; message.value = '参加登録があるため、個人戦とチーム戦は切り替えられません。'; return }
    if (code === 'contest_team_size_locked') { section.value = 'participation'; message.value = '人数の上限は、今いちばん人数の多いチームより少なくできません。'; return }
    message.value = status === 404 ? noLongerSetter : status === 409 ? '保存できませんでした。問題の公開状態・他コンテストへの登録・テストケースを確認してください。参加登録した人が作者・テスターの問題と、作成者の誰も作者・テスターではない問題は出題できません。開催開始や別画面での更新があった場合は再読み込みが必要です。' : status === 400 ? '入力内容を確認してください。' : '保存を確認できませんでした。作成したコンテスト一覧を確認してから再度お試しください。'
  } finally { busy.value = false }
}
</script>
<template>
  <div class="author-page contest-editor">
    <header class="editor-topbar">
      <NuxtLink class="wordmark" to="/my" aria-label="ShareOJ マイページ">Share<span>OJ</span><span class="wordmark-beta">(β)</span></NuxtLink>
      <h1 class="visually-hidden">{{ contestId ? 'コンテスト編集' : 'コンテスト作成' }}</h1>
      <div v-show="section === 'description'" class="editor-view-switch" aria-label="表示の切り替え">
        <button type="button" class="editor-button" :aria-pressed="mode === 'edit'" aria-label="編集" title="編集" @click="mode = 'edit'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg></button>
        <button type="button" class="editor-button split-button" :aria-pressed="mode === 'split'" aria-label="分割" title="分割" @click="mode = 'split'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M12 3v18" /></svg></button>
        <button type="button" class="editor-button" :aria-pressed="mode === 'preview'" aria-label="プレビュー" title="プレビュー" @click="mode = 'preview'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg></button>
      </div>
      <div class="author-actions">
        <button type="submit" form="contest-form" class="editor-button save-button" :class="{ primary: published }" :disabled="!ready || locked || busy" :aria-busy="busy" :aria-label="busy ? '保存中' : published ? '変更を保存' : '下書きを保存'" title="保存（Ctrl+S / ⌘S）" aria-keyshortcuts="Control+s Meta+s">
          <span :class="{ 'save-label-hidden': busy }">保存</span><span v-if="busy" class="save-spinner" aria-hidden="true" />
        </button>
        <button v-if="!published" type="button" class="editor-button primary" :disabled="!ready || locked || busy" aria-label="コンテストを公開" title="検証して公開" @click="publish">公開</button>
      </div>
    </header>
    <div class="author-body" :data-sidebar-expanded="sidebarExpanded">
      <aside class="editor-sidebar" aria-label="コンテスト作成サイドバー">
        <button type="button" class="editor-button editor-sidebar-toggle" :aria-expanded="sidebarExpanded" aria-controls="contest-section-nav" :aria-label="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" :title="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" @click="sidebarExpanded = !sidebarExpanded">
          <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /><path :d="sidebarExpanded ? 'm15 9-3 3 3 3' : 'm13 9 3 3-3 3'" /></svg>
        </button>
        <nav id="contest-section-nav" class="editor-section-nav" aria-label="コンテスト作成メニュー">
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'description' ? 'page' : undefined" aria-label="タイトル・説明" title="タイトル・説明" @click="section = 'description'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg><span class="editor-sidebar-label">タイトル・説明</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'problems' ? 'page' : undefined" aria-label="問題・配点" title="問題・配点" @click="section = 'problems'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m3 6 2 2 3-4m-5 9 2 2 3-4m-5 9 2 2 3-4M12 6h9M12 13h9M12 20h9" /></svg><span class="editor-sidebar-label">問題・配点</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'schedule' ? 'page' : undefined" aria-label="期間・ペナルティ" title="期間・ペナルティ" @click="section = 'schedule'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></svg><span class="editor-sidebar-label">期間・ペナルティ</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'participation' ? 'page' : undefined" aria-label="参加形式" title="参加形式" @click="section = 'participation'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="9" cy="8" r="3" /><path d="M3 20a6 6 0 0 1 12 0M16 4a3 3 0 0 1 0 6M21 20a6 6 0 0 0-4-5.7" /></svg><span class="editor-sidebar-label">参加形式</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'manage' ? 'page' : undefined" aria-label="コンテスト管理" title="コンテスト管理" @click="section = 'manage'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 3 .6-2h4.8l.6 2 2 1.2 2.1-.5 2.4 4.2-1.5 1.5v2.3l1.5 1.5-2.4 4.2-2.1-.5-2 1.2-.6 2H9l-.6-2-2-1.2-2.1.5-2.4-4.2 1.5-1.5V9.4L1.9 7.9l2.4-4.2 2.1.5Z" transform="translate(0 1)" /><circle cx="12" cy="12" r="3" /></svg><span class="editor-sidebar-label">コンテスト管理</span></button>
        </nav>
      </aside>
      <form id="contest-form" ref="form" class="editor-main" novalidate @submit.prevent="save">
        <div class="editor-notices"><p v-if="message" class="editor-error" role="alert">{{ message }}</p><noscript><p class="editor-error">編集と保存には JavaScript を有効にしてください。</p></noscript></div>
        <div v-show="section === 'description'" class="author-edit-content" data-section="description">
          <div class="author-fields"><div class="title-field"><div class="field-heading"><label for="contest-title">コンテストタイトル</label></div><input id="contest-title" v-model="title" required maxlength="120" placeholder="コンテストのタイトル" :disabled="!ready || locked || busy"></div></div>
    <div ref="workspace" class="author-workspace" :class="{ 'is-resizing': resizing }" :data-mode="mode" :style="{ '--editor-left': `${splitPercent}fr`, '--editor-right': `${100 - splitPercent}fr` }">
      <section id="source-pane" class="source-pane" aria-label="Markdown 編集">
        <div class="pane-heading"><div class="source-heading-label"><label id="contest-description-label" for="contest-description" @click="editor?.focus()">説明（Markdown）</label><NuxtLink class="source-guide-link" to="/docs/markdown" target="_blank" rel="noopener noreferrer" aria-label="Markdown・数式の書き方" title="Markdown・数式の書き方"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M9.5 9a2.5 2.5 0 0 1 5 .5c0 1.5-2.5 2-2.5 3.5M12 16h.01" /></svg></NuxtLink></div><span>{{ description.length.toLocaleString('en-US') }} / 100,000</span></div>
        <div class="editor-toolbar" aria-label="記法を挿入">
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n## 見出し\n')" aria-label="見出し" title="見出し"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 5v14M19 5v14M5 12h14" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('**強調**')" aria-label="太字" title="太字"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path stroke-width="2.4" d="M6 12h7a4 4 0 0 1 0 8H6V4h6a4 4 0 0 1 0 8" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet(mathSnippet)" aria-label="数式" title="数式"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M19 4H6l7 8-7 8h13" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n```text\nコード\n```\n')" aria-label="コード" title="コード"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m8 6-6 6 6 6m8-12 6 6-6 6m-3-14-2 16" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n:::details タイトル\n内容\n:::\n')" aria-label="折りたたみ" title="折りたたみ"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 8 3 3 3-3M7 15h10" /></svg></button>
          <button type="button" :disabled="!ready || busy || editor?.uploading" :aria-expanded="editor?.showImages ?? false" @click="editor?.toggleImages()" aria-label="画像" title="画像"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8" cy="8" r="1.5" /><path d="m21 15-5-5L5 21M3 16l4-4 4 4" /></svg></button>
          <EditorSettings kind="markdown" />
        </div>
        <MarkdownSourceEditor id="contest-description" ref="editor" v-model="description" label-id="contest-description-label" :disabled="!ready || busy" @scroll="editorScrolled" />
      </section>
      <div class="split-handle" role="separator" tabindex="0" aria-label="編集欄とプレビューの幅を調整" aria-orientation="vertical" aria-controls="source-pane preview-pane" :aria-valuenow="Math.round(splitPercent)" :aria-valuetext="`編集欄 ${Math.round(splitPercent)}%、プレビュー ${100 - Math.round(splitPercent)}%`" :aria-valuemin="30" :aria-valuemax="70" title="ドラッグで幅を調整・ダブルクリックで均等に戻す" @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="resizing = false" @keydown="resizeWithKeyboard" @dblclick="setSplit(50)" />
      <section id="preview-pane" class="preview-pane" aria-label="コンテストのプレビュー">
        <div class="pane-heading"><h2>プレビュー</h2></div>
        <div ref="preview" class="preview-document contest-preview" @scroll="previewScrolled">
          <p class="preview-title">{{ title || 'コンテストタイトル' }}</p>
          <ProblemMarkdown v-if="description.trim()" :source="description" live />
          <p v-else class="muted">説明を書くと、ここにプレビューが表示されます。</p>
        </div>
      </section>
    </div>
    </div>
        <section v-show="section === 'problems'" class="problem-management" data-section="problems" aria-labelledby="contest-problems-title">
          <div class="management-content">
            <header><h2 id="contest-problems-title">問題・配点</h2><p class="muted">自分の未公開問題から選び、出題順と配点を設定します。<template v-if="setterList.length > 1">ほかの作成者の問題は、その作成者が追加します。</template></p></header>
            <fieldset :disabled="!ready || locked || busy" class="problem-sections">
              <section aria-labelledby="selected-problems-title"><h3 id="selected-problems-title">出題順・配点 <span class="muted">{{ selected.length }} 問 · 合計 {{ totalPoints }} 点</span></h3>
                <p v-if="!selected.length" class="muted">下の「問題を追加」から出題する問題を選んでください。</p>
                <p v-if="selected.length > 1" id="reorder-hint" class="muted reorder-hint">左端のつまみをドラッグして並べ替えます。キーボードでは、つまみを選んで↑↓キーで動かします。</p>
                <ol ref="list" class="selected" :data-dragging="drag ? '' : undefined">
                  <li v-for="(p, index) in selected" :key="p.id" :data-id="p.id" :class="{ dragging: drag?.from === index }" :style="rowStyle(index)">
                    <button type="button" class="drag-handle" :aria-label="`${problemName(p.title)}を並べ替え`" aria-describedby="reorder-hint" title="ドラッグで並べ替え" @pointerdown="startDrag($event, index)" @pointermove="updateDrag($event.clientY)" @pointerup="endDrag(true)" @pointercancel="endDrag(false)" @lostpointercapture="endDrag(false)" @keydown="moveWithKeyboard($event, index)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="9" cy="6" r="1" /><circle cx="15" cy="6" r="1" /><circle cx="9" cy="12" r="1" /><circle cx="15" cy="12" r="1" /><circle cx="9" cy="18" r="1" /><circle cx="15" cy="18" r="1" /></svg></button>
                    <span class="problem-label">{{ problemLabel(slot(index)) }}</span>
                    <div class="problem-entry"><span><strong>{{ problemName(p.title) }}</strong><span v-if="problemOwner(p.id)" class="problem-role">作者 {{ problemOwner(p.id) }}</span></span><code class="problem-uuid">{{ p.id }}</code><p v-if="problemIssue(p.id)" class="problem-issue">{{ problemIssue(p.id) }}</p></div>
                    <div class="selected-controls">
                      <label class="points-field" :for="`points-${p.id}`">配点 <input :id="`points-${p.id}`" v-model.number="p.points" type="number" min="1" max="1000000" step="1" required> 点</label>
                      <button class="editor-button remove-button" type="button" :aria-label="`${problemName(p.title)}を外す`" title="外す" @click="remove(index)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg></button>
                    </div>
                  </li>
                </ol>
                <p class="visually-hidden" aria-live="polite">{{ reorderStatus }}</p>
              </section>
              <section aria-labelledby="available-problems-title"><h3 id="available-problems-title">問題を追加</h3>
                <p v-if="ready && !available.length"><NuxtLink to="/problems/new?fresh=1" target="_blank" rel="noopener noreferrer">問題を作成・保存 ↗</NuxtLink>してから、この画面を再読み込みしてください。</p>
                <template v-else>
                  <input v-model="query" class="problem-search" type="search" aria-label="問題を検索" placeholder="タイトルまたはUUIDで検索" autocomplete="off" aria-describedby="problem-search-hint" aria-controls="problem-candidates" @keydown.enter.prevent="addFirstCandidate">
                  <p id="problem-search-hint" class="muted search-hint">自分の未公開問題と、テスターとして参加している未公開問題から探します。<template v-if="published && setterList.length > 1">このコンテストに入っている、ほかの作成者の問題も出ます。</template>他のコンテストに登録済みの問題は出ません。Enterで先頭の候補を追加します。</p>
                  <label class="addable-filter"><input v-model="onlyAddable" type="checkbox"> 追加できる問題だけ表示</label>
                  <p class="muted search-count" role="status">{{ candidates.length }} 件{{ query.trim() ? 'が一致' : '' }}{{ matches.length > candidates.length ? `（追加済み・追加できない ${matches.length - candidates.length} 件を非表示）` : '' }}</p>
                  <ul id="problem-candidates" class="candidates">
                    <li v-for="p in candidates" :key="p.id">
                      <div class="problem-entry"><span>{{ problemName(p.title) }}<span v-if="tested.has(p.id)" class="problem-role">テスター</span><span v-else-if="problemOwner(p.id)" class="problem-role">作者 {{ problemOwner(p.id) }}</span></span><code class="problem-uuid">{{ p.id }}</code><p v-if="problemIssue(p.id)" class="problem-issue">{{ problemIssue(p.id) }}</p></div>
                      <button class="editor-button" type="button" :disabled="!addable(p.id)" :aria-label="`${problemName(p.title)}${isSelected(p.id) ? 'は追加済み' : 'を追加'}`" @click="add(p)">{{ isSelected(p.id) ? '追加済み' : '追加' }}</button>
                    </li>
                  </ul>
                </template>
              </section>
            </fieldset>
            <p class="selection-note muted">問題の保存内容は、開始後もコンテストに自動で反映されます。更新後の提出から新しい内容で採点します。受付済みの提出を採点し直すには、問題管理からリジャッジしてください。</p>
          </div>
        </section>
        <section v-show="section === 'schedule'" class="problem-management" data-section="schedule" aria-labelledby="contest-schedule-title">
          <div class="management-content">
            <header><h2 id="contest-schedule-title">期間・ペナルティ</h2><p class="muted">開催時間と誤答ペナルティを設定します。</p></header>
            <fieldset :disabled="!ready || locked || busy">
              <section class="settings-section"><h3>開催時間</h3><p class="muted">日時は {{ timezone || '端末のタイムゾーン' }} で入力します。公開ページでは日本時間で表示します。</p>
                <div class="dates"><div><label for="contest-start">開始日時</label><input id="contest-start" v-model="startsAt" type="datetime-local" required></div><div><label for="contest-duration">コンテスト時間（分）</label><input id="contest-duration" v-model.number="durationMinutes" type="number" required min="1" :max="maxDurationMinutes" step="1"></div></div>
                <p class="muted end-time">{{ endLabel }}</p>
              </section>
              <section class="settings-section"><h3>誤答ペナルティ</h3><p class="muted">正解した問題の、初回正解前の誤答だけに加算します。コンパイルエラーは対象外です。</p><label for="contest-penalty">誤答ペナルティ（分）</label><input id="contest-penalty" v-model.number="penaltyMinutes" type="number" required min="0" max="1440" step="1"><p class="muted penalty-note">0分でペナルティなしにできます。</p></section>
              <section class="settings-section"><h3>順位表の凍結</h3><InlineSwitch v-model="freezeOn" class="settings-switch freeze-switch" describedby="contest-freeze-note">順位表の凍結を行う</InlineSwitch><p id="contest-freeze-note" class="muted">終了前の指定した時間の提出は、終了後に「結果を公開」を押すまで順位表で結果を伏せます。</p><template v-if="freezeOn"><label for="contest-freeze">凍結する時間（終了前の分）</label><input id="contest-freeze" v-model.number="freezeMinutes" type="number" min="1" :max="Number(durationMinutes) || undefined" step="1" required></template></section>
            </fieldset>
            <p class="muted">開始後は問題セット・配点・開催期間・ペナルティ・凍結を変更できません。問題と解説は終了後に自動公開されます。</p>
          </div>
        </section>
        <section v-show="section === 'participation'" class="problem-management" data-section="participation" aria-labelledby="contest-participation-title">
          <div class="management-content">
            <header><h2 id="contest-participation-title">参加形式</h2><p class="muted">個人戦かチーム戦かと、オンサイト参加の区分を設定します。</p></header>
            <fieldset :disabled="!ready || locked || busy">
              <section class="settings-section"><h3>個人戦・チーム戦</h3>
                <div class="team-mode"><SegmentedChoice v-model="teamMode" legend="個人戦・チーム戦" name="contest-team-mode" :options="teamModes" hide-legend /></div>
                <p class="muted">チーム戦では、参加者がチームを作るか参加コードでチームに入り、順位表にはチームごとに並びます。個人で参加した人も1人で並びます。参加登録があると切り替えられません。</p>
                <template v-if="teamMode === 'team'">
                  <InlineSwitch v-model="teamSizeOn" class="settings-switch" describedby="contest-team-size-note">人数の上限を設ける</InlineSwitch>
                  <p id="contest-team-size-note" class="muted">オフのときも、1チーム{{ maxTeamMembers }}人までです。</p>
                  <template v-if="teamSizeOn"><label for="contest-team-size">1チームの人数の上限（人）</label><input id="contest-team-size" v-model.number="maxTeamSize" type="number" min="2" :max="maxTeamMembers" step="1" required></template>
                </template>
              </section>
              <section class="settings-section"><h3>オンサイト区分</h3><InlineSwitch :model-value="onsiteOn" class="settings-switch" describedby="contest-onsite-note" @update:model-value="setOnsite">オンサイト区分を設ける</InlineSwitch><p id="contest-onsite-note" class="muted">会場の合言葉を入れて参加した人をオンサイト参加とし、順位表と提出一覧で絞り込めるようにします。</p>
                <!-- After the start the editor does not read the passphrase; it lives in the contest page's tools. -->
                <p v-if="onsiteOn && settersOnly" class="muted">会場の合言葉は、コンテストのページの「参加者の管理」で確認できます。</p>
                <template v-else-if="onsiteOn"><label for="contest-passphrase">会場の合言葉</label><VenuePassphraseInput id="contest-passphrase" v-model="passphrase" required aria-describedby="contest-passphrase-note" /><p id="contest-passphrase-note" class="muted passphrase-note">{{ venuePassphraseRule }}で、大文字と小文字は区別します。作成者だけが見られます。会場の参加者に配ってください。</p></template>
              </section>
            </fieldset>
            <p class="muted">開始後は参加形式とオンサイト区分を変更できません。会場の合言葉の差し替えと、オープン参加の人とチームをオンサイト参加にする操作は、コンテストのページの「参加者の管理」でできます。</p>
          </div>
        </section>
        <section v-show="section === 'manage'" class="problem-management" data-section="manage" aria-labelledby="contest-manage-title">
          <div class="management-content">
            <header><h2 id="contest-manage-title">コンテスト管理</h2><p class="manage-problem-title">{{ title.trim() || '無題のコンテスト' }}</p><p class="muted">{{ settersOnly ? '公開済み（開始後）' : published ? '公開済み（開催予定）' : '下書き（未公開）' }}</p></header>
            <section class="management-row readiness-row" aria-labelledby="contest-readiness-title"><div>
              <h3 id="contest-readiness-title">状態</h3>
              <p v-if="settersOnly">{{ resultsFinal ? '結果が確定したため、共同作成者の招待はできません。解除と脱退はできます。' : '開始後は設定を変更できません。共同作成者の招待と解除はできます。' }}</p>
              <p v-else-if="published">公開済みです。開始前なら内容を変更でき、保存すると公開ページに反映されます。</p>
              <p v-else-if="!version">保存すると、公開できるかを確認できます。</p>
              <template v-else>
                <p v-if="showIssues && draftIssues.length" class="editor-error" role="alert">公開するには、次の項目を直して保存してください。</p>
                <div class="readiness-heading"><span class="readiness-badge" :data-ready="!draftIssues.length">{{ draftIssues.length ? `×公開できません（${draftIssues.length} 項目）` : '✅公開できます' }}</span><span>{{ dirty ? '保存済みの内容で判定しています。未保存の変更は含みません' : '保存済みの内容で判定しています' }}</span></div>
                <ul v-if="draftIssues.length" class="readiness-issues" aria-label="足りない項目">
                  <li v-for="code in draftIssues" :key="code"><span>{{ issueText(code) }}</span><button type="button" class="editor-button" @click="section = issueSection(code)">{{ sectionNames[issueSection(code)] }}を開く</button></li>
                </ul>
              </template>
            </div></section>
            <section v-if="!published" class="management-row"><div><h3>公開</h3><p>公開するまで、このコンテストは作成者だけが見られる下書きです。公開すると、保存した内容で条件を確かめてからコンテスト一覧に載せ、参加登録を受け付けます。</p></div><button type="button" class="editor-button primary" :disabled="!ready || locked || busy" @click="publish">公開する</button></section>
            <section v-else class="management-row"><div><h3>公開ページ</h3><p>参加者に見える内容を確認できます。</p><NuxtLink :to="`/contests/${shortId(id)}`" target="_blank">公開ページを見る</NuxtLink></div></section>
            <!-- Only a setter sees who sets the contest; a locked editor of someone else's contest never loads them. -->
            <section v-if="ready || settersOnly" class="management-row setters-row" aria-labelledby="contest-setters-title"><div class="setters">
              <h3 id="contest-setters-title">共同作成者</h3>
              <p>招待リンクから加わった共同作成者は、オーナーと同じように設定の編集、下書きの保存と公開、問題の追加と削除ができます。共同作成者を外すことと下書きの削除は、オーナーだけができます。</p>
              <ul ref="setterListElement" class="setter-list" aria-label="作成者" tabindex="-1">
                <li v-for="(handle, index) in setterList" :key="handle">
                  <span class="setter-name"><UserLink :handle="handle" /><span class="problem-role">{{ index ? '共同作成者' : 'オーナー' }}</span></span>
                  <button v-if="isOwner && index" type="button" class="editor-button" :disabled="!canManageSetters" :aria-label="`${handle}を共同作成者から外す`" @click="removeSetter(handle)">外す</button>
                  <button v-else-if="!isOwner && handle === me" type="button" class="editor-button danger" :disabled="!canManageSetters" @click="leaveContest">共同作成者から抜ける</button>
                </li>
              </ul>
              <template v-if="setterLink"><label for="setter-link">招待リンク</label><div class="setter-link-field"><input id="setter-link" :value="setterLink" readonly @focus="($event.target as HTMLInputElement).select()"><button type="button" class="editor-button setter-link-copy" :aria-label="setterLinkCopied ? 'コピーしました' : 'リンクをコピー'" :title="setterLinkCopied ? 'コピーしました' : 'リンクをコピー'" @click="copySetterLink"><svg v-if="setterLinkCopied" class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5L19 8" /></svg><svg v-else class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V4H4v12h4" /></svg></button></div></template>
              <p v-if="setterMessage" role="status">{{ setterMessage }}</p>
            </div><button v-if="!resultsFinal" type="button" class="editor-button" :disabled="!canManageSetters" :aria-busy="setterAction === 'link'" @click="setterInvitation(!!setterLink)">{{ setterAction === 'link' ? '発行中…' : setterLink ? 'リンクを作り直す' : 'リンクを発行' }}</button></section>
            <section v-if="!published && version && isOwner" class="management-row"><div><h3>下書きの削除</h3><p>この下書きを削除します。この操作は取り消せません。</p></div><button type="button" class="editor-button danger" :disabled="busy" @click="removeDraft">下書きを削除</button></section>
          </div>
        </section>
      </form>
    </div>
    <footer class="draft-status"><LoadingSpinner v-if="!ready && !message && !settersOnly" inline label="コンテストを読み込み中" /><span v-else role="status">{{ status }}</span><NuxtLink to="/my?tab=contests">自分のコンテスト</NuxtLink></footer>
  </div>
</template>
<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4
 * modern-minimal · Workbench · existing Plain tokens and editor controls */
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: clip; clip-path: inset(50%); white-space: nowrap; }
.editor-topbar .author-actions { width: auto; flex-shrink: 0; }
.contest-editor .author-fields { grid-template-columns: minmax(0, 1fr); }
.draft-status > :last-child { display: inline-flex; white-space: nowrap; }
fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
.management-content > header h2 { font-size: 1.5rem; margin-bottom: 12px; }
.management-content h3 { font-size: 1rem; font-weight: 600; margin-bottom: 12px; }
.management-content h3 span { display: block; font-size: .8125rem; font-weight: 400; margin-top: 4px; }
.management-content p, .management-content label { font-size: .875rem; }
.problem-sections { display: grid; gap: 40px; }
.selected, .candidates { list-style: none; padding: 0; margin: 0; border-top: 1px solid var(--color-line); }
.selected li, .candidates li { padding: 12px 0; border-bottom: 1px solid var(--color-line); }
.selected { position: relative; }
.selected li { --handle-size: 32px; display: grid; grid-template-columns: var(--handle-size) 1.5rem minmax(0, 1fr); grid-template-areas: "handle label body" "controls controls controls"; align-items: center; gap: 8px; background: var(--color-paper); }
.selected[data-dragging] { user-select: none; }
.selected[data-dragging] li { transition: transform .15s ease; }
.selected li.dragging { position: relative; z-index: 1; transition: none; box-shadow: 0 6px 20px rgb(0 0 0 / .18); }
.drag-handle { grid-area: handle; align-self: start; display: inline-flex; align-items: center; justify-content: center; width: var(--handle-size); height: var(--handle-size); padding: 0; border: 0; border-radius: 4px; background: none; color: var(--color-muted); cursor: grab; touch-action: none; }
.drag-handle:hover:not(:disabled) { background: var(--color-surface); color: var(--color-ink); }
.drag-handle:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 0; }
.drag-handle:disabled { cursor: default; }
.drag-handle circle { fill: currentColor; }
.dragging .drag-handle { cursor: grabbing; }
.reorder-hint { margin-bottom: 8px; }
.candidates li { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.problem-label { grid-area: label; align-self: start; color: var(--color-muted); font-family: var(--font-code); font-weight: 700; }
.selected .problem-entry { grid-area: body; }
.problem-entry { display: grid; min-width: 0; }
.problem-entry > :first-child { overflow-wrap: anywhere; }
.problem-uuid { color: var(--color-muted); font-size: .75rem; overflow-wrap: anywhere; }
.problem-role { margin-inline-start: 8px; padding: 1px 6px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); font-size: .75rem; white-space: nowrap; }
.problem-issue { margin: 4px 0 0; color: var(--color-error); }
.selected-controls { grid-area: controls; display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; }
.points-field { display: inline-flex; align-items: center; gap: 8px; white-space: nowrap; }
.remove-button { width: 36px; min-height: 36px; display: inline-flex; align-items: center; justify-content: center; }
.candidates button { flex-shrink: 0; min-width: 5.5em; }
.problem-search { margin-bottom: 8px; }
.search-hint, .search-count { margin-bottom: 8px; }
.addable-filter { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 4px; cursor: pointer; }
.addable-filter input { accent-color: var(--color-accent); }
.selection-note { margin-top: 24px; }
.management-content input:not([type=checkbox]) { width: 100%; min-width: 0; min-height: 44px; padding: 8px 10px; font: inherit; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-ink); background: var(--color-paper); }
.management-content input[type=number] { max-width: 128px; }
.management-content .points-field input { width: 96px; }
.management-content input:hover:not(:disabled) { border-color: var(--color-muted); }
.settings-section { padding-block: 24px; border-top: 1px solid var(--color-line); }
.settings-section label { display: block; margin-bottom: 8px; }
/* InlineSwitch is a label too; keep its own inline-flex layout instead of the block labels above. */
.settings-section .settings-switch { display: inline-flex; margin-bottom: 4px; }
.settings-section .settings-switch + p { margin-bottom: 12px; }
.team-mode { max-width: 24rem; margin-bottom: 8px; }
.management-content #contest-passphrase { max-width: 24rem; font-family: var(--font-code); }
.passphrase-note { margin-top: 8px; }
.dates { display: grid; gap: 20px; }
.penalty-note, .end-time { margin-top: 8px; }
.management-row h3 { margin: 0 0 4px; }
.readiness-row > div { display: grid; flex: 1; gap: 8px; min-width: 0; }
.readiness-row .editor-error { margin: 0; color: var(--color-ink); }
.readiness-heading { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.readiness-heading > span:last-child { color: var(--color-muted); font-size: .8125rem; }
.readiness-badge { padding: 2px 8px; border: 1px solid currentColor; border-radius: 4px; color: var(--color-error); font-size: .8125rem; font-weight: 600; }
.readiness-badge[data-ready="true"] { color: var(--color-accent); }
.readiness-issues { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
.readiness-issues li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 10px; border: 1px solid var(--color-line); border-radius: 4px; font-size: .875rem; }
.readiness-issues li > span { min-width: 0; overflow-wrap: anywhere; }
.setters { display: grid; flex: 1; gap: 8px; min-width: 0; }
.setters-row { align-items: flex-start; }
.setter-list { display: grid; margin: 0; padding: 0; list-style: none; border-top: 1px solid var(--color-line); }
.setter-list li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-height: 44px; padding: 4px 0; border-bottom: 1px solid var(--color-line); font-size: .875rem; }
.setter-name { display: inline-flex; align-items: center; min-width: 0; overflow-wrap: anywhere; }
.setters label { margin-top: 4px; font-size: .75rem; color: var(--color-muted); }
.setter-link-field { display: flex; gap: 8px; }
.management-content #setter-link { flex: 1; min-width: 0; font-family: var(--font-code); font-size: .8125rem; }
.setter-link-copy { display: inline-flex; align-items: center; justify-content: center; width: 44px; padding: 0; flex-shrink: 0; }
@media (min-width: 48rem) { .selected li { grid-template-columns: var(--handle-size) 1.5rem minmax(0, 1fr) auto; grid-template-areas: "handle label body controls"; } }
@media (prefers-reduced-motion: reduce) { .selected[data-dragging] li { transition: none; } }
@media (min-width: 60rem) { .dates { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (pointer: coarse) { .remove-button { width: 44px; min-height: 44px; } .selected li { --handle-size: 44px; } }
</style>
