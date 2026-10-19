import type { PublicProblem } from './problem'
import { shortId } from '../utils/short-id'
// hasEditorial is present only when the viewer may read the editorial now. False means none is registered.
// unmanaged tells setters before the start that no current setter authors or tests the problem, so saving fails.
export type ContestProblem = { id: string, points: number, title?: string, timeLimitMs?: string, memoryLimitMb?: string, solved?: boolean, hasEditorial?: boolean, unmanaged?: boolean }
// author is the owner; authors lists every setter, owner first. staff, setter and owner describe the viewer (ADR 0022).
// excluded: the viewer is recorded as former staff of this contest; public responses always say false.
export type Contest = {
  id: string, author: string, authors?: string[], problemAuthors: string[], testers: string[], title: string, description: string,
  startsAt: string, endsAt: string, penaltyMinutes: number, version: number,
  status: 'scheduled' | 'running' | 'ended', canEdit: boolean, participating: boolean, official: boolean, canViewSubmissions: boolean,
  staff?: boolean, setter?: boolean, owner?: boolean, excluded?: boolean,
  // Standings freeze (ADR 0023): freezeAt is null without a freeze; resultsFinal is true once ended without a freeze or revealed.
  freezeMinutes?: number | null, freezeAt?: string | null, revealed?: boolean, revealedAt?: string | null, resultsFinal?: boolean,
  canViewLiveStandings?: boolean, canRevealResults?: boolean,
  // Team contests and the onsite division (ADR 0024). maxTeamSize is null without a limit of the contest's own. team is
  // the viewer's team, null for someone registered alone; onsite tells whether the viewer, or their team, is onsite.
  teamMode?: boolean, maxTeamSize?: number | null, onsiteDivision?: boolean, team?: TeamSummary | null, onsite?: boolean,
  problems: ContestProblem[],
}
// A team as everyone sees it in the standings; members are handles in the order they joined.
export type TeamSummary = { id: string, name: string, members: string[], onsite: boolean }
// The viewer's own team, with the join code only members see.
export type MyTeam = TeamSummary & { code: string }
// One row of the setters' list: a team, or a participant in no team (everyone in an individual contest, and those who
// joined a team contest alone). onsiteVia is empty while open.
export type Entrant = { handle: string, team: TeamSummary | null, onsite: boolean, onsiteAt: string | null, onsiteVia: 'passphrase' | 'setter' | '', createdAt: string }
// The onsite division and its passphrase, for setters only.
export type Venue = { onsiteDivision: boolean, passphrase: string | null }
// The participation body: an empty one joins alone as open, in team contests too.
export type JoinBody = { team?: { action: 'create', name: string } | { action: 'join', code: string }, onsitePassphrase?: string }
// Setters, owner first; responses from before co-setters name the owner alone.
export const contestAuthors = (contest: { author: string, authors?: string[] }) => contest.authors?.length ? contest.authors : [contest.author]
export type ContestList = { items: Contest[], hasMore: boolean }
// A contest before publication: any field may be missing until the server reports no issues.
export type ContestDraft = {
  title: string, description: string, startsAt: string | null, durationMinutes: number | null, penaltyMinutes: number | null,
  freezeMinutes?: number | null,
  teamMode?: boolean, maxTeamSize?: number | null, onsiteDivision?: boolean, onsitePassphrase?: string | null,
  problems: { id: string, points: number | null }[],
}
// problems describes the listed problems some current setter authors or tests, so co-setters see each other's titles.
export type DraftProblem = { id: string, title: string, author: string, issues: string[] }
export type ContestDraftResult = { id: string, version: number, updatedAt: string, draft: ContestDraft, issues: string[], problems?: DraftProblem[], authors?: string[], owner?: boolean }
export type ContestDraftList = { items: { id: string, title: string, author?: string, startsAt: string | null, updatedAt: string }[], hasMore: boolean }
// A co-setter link preview; id is the draft's or contest's id, which the two share. closed: the results are final.
export type SetterInvitation = { id: string, title: string, author: string, published: boolean, joined: boolean, closed?: boolean }
// Team contests rank teams beside those who joined alone: a team's row has an empty handle and team names it, and an
// individual's row has team null. overallRank is the rank in the whole table, which differs from rank only in the onsite
// table.
export type Standing = {
  rank: number, overallRank?: number, handle: string, team?: TeamSummary | null, onsite?: boolean, points: number, timeMs: number,
  problems: Record<string, { points: number, wrong: number, pending: number, frozen?: number, acceptedAt?: string }>,
}
// authors are the contest's setters; canReadEditorial is the server's answer for this viewer.
export type ContestProblemDetail = Omit<PublicProblem, 'timeLimitMs' | 'memoryLimitMb'> & { timeLimitMs: string, memoryLimitMb: string, authors?: string[], canReadEditorial?: boolean }
export const contestStatus = { scheduled: '開催予定', running: '開催中', ended: '終了' }
export const contestViews = { overview: '概要', problems: '問題', standings: '順位表', submissions: '提出一覧', editorials: '解説一覧' }
export type ContestView = keyof typeof contestViews
export function contestView(value: unknown): ContestView {
  return value === 'problems' || value === 'standings' || value === 'submissions' || value === 'editorials' ? value : 'overview'
}
export function contestDate(value: string) {
  return new Intl.DateTimeFormat('ja-JP', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
}
// When the standings freeze starts, in Japan. A contest that crosses midnight there shows the date as well, since the
// time alone could mean either day.
export function contestFreezeTime(contest: Pick<Contest, 'startsAt' | 'endsAt' | 'freezeAt'>) {
  if (!contest.freezeAt) return ''
  const day = (value: string) => new Intl.DateTimeFormat('ja-JP', { dateStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
  const date = day(contest.startsAt) !== day(contest.endsAt) ? { month: 'long', day: 'numeric' } as const : {}
  return new Intl.DateTimeFormat('ja-JP', { ...date, hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' }).format(new Date(contest.freezeAt))
}
// Others do not see the frozen submissions until the results are published (ADR 0023); empty when nothing is hidden.
// Teammates see them too (ADR 0024).
export function frozenSubmissionsNote(contest: Pick<Contest, 'startsAt' | 'endsAt' | 'freezeAt' | 'resultsFinal' | 'teamMode'>) {
  const viewers = contest.teamMode ? '本人とチームメイト、スタッフ' : '本人とスタッフ'
  return contest.freezeAt && !contest.resultsFinal ? `${contestFreezeTime(contest)}以降の提出は、結果の公開まで${viewers}だけが見られます。` : ''
}
// A, B, …, Z, AA, AB, … in contest order.
export function problemLabel(index: number): string {
  return index < 26 ? String.fromCharCode(65 + index) : problemLabel(Math.floor(index / 26) - 1) + problemLabel(index % 26)
}
// The inverse of problemLabel; -1 for anything it never produces.
export function labelIndex(label: unknown) {
  if (typeof label !== 'string' || !/^[A-Z]{1,3}$/.test(label)) return -1
  let index = 0
  for (const char of label) index = index * 26 + char.charCodeAt(0) - 64
  return index - 1
}
export const contestProblemPath = (contestId: string, index: number) => `/contests/${shortId(contestId)}/${problemLabel(index)}`
