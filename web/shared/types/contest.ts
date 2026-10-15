import type { PublicProblem } from './problem'
import { shortId } from '../utils/short-id'
// hasEditorial is present only when the viewer may read the editorial now. False means none is registered.
export type ContestProblem = { id: string, points: number, title?: string, timeLimitMs?: string, memoryLimitMb?: string, solved?: boolean, hasEditorial?: boolean }
export type Contest = {
  id: string, author: string, problemAuthors: string[], testers: string[], title: string, description: string,
  startsAt: string, endsAt: string, penaltyMinutes: number, version: number,
  status: 'scheduled' | 'running' | 'ended', canEdit: boolean, participating: boolean, official: boolean, canViewSubmissions: boolean,
  problems: ContestProblem[],
}
export type ContestList = { items: Contest[], hasMore: boolean }
// A contest before publication: any field may be missing until the server reports no issues.
export type ContestDraft = {
  title: string, description: string, startsAt: string | null, durationMinutes: number | null, penaltyMinutes: number | null,
  problems: { id: string, points: number | null }[],
}
export type ContestDraftResult = { id: string, version: number, updatedAt: string, draft: ContestDraft, issues: string[] }
export type ContestDraftList = { items: { id: string, title: string, startsAt: string | null, updatedAt: string }[], hasMore: boolean }
export type Standing = {
  rank: number, handle: string, points: number, timeMs: number,
  problems: Record<string, { points: number, wrong: number, pending: number, acceptedAt?: string }>,
}
export type ContestProblemDetail = Omit<PublicProblem, 'timeLimitMs' | 'memoryLimitMb'> & { timeLimitMs: string, memoryLimitMb: string }
export const contestStatus = { scheduled: '開催予定', running: '開催中', ended: '終了' }
export const contestViews = { overview: '概要', problems: '問題', standings: '順位表', submissions: '提出一覧', editorials: '解説一覧' }
export type ContestView = keyof typeof contestViews
export function contestView(value: unknown): ContestView {
  return value === 'problems' || value === 'standings' || value === 'submissions' || value === 'editorials' ? value : 'overview'
}
export function contestDate(value: string) {
  return new Intl.DateTimeFormat('ja-JP', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
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
