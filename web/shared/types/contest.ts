import type { PublicProblem } from './problem'
export type ContestProblem = { id: string, points: number, title?: string, solved?: boolean }
export type Contest = {
  id: string, author: string, title: string, description: string,
  startsAt: string, endsAt: string, penaltyMinutes: number, version: number,
  status: 'scheduled' | 'running' | 'ended', canEdit: boolean, official: boolean, canViewSubmissions: boolean,
  problems: ContestProblem[],
}
export type ContestList = { items: Contest[], hasMore: boolean }
export type Standing = {
  rank: number, handle: string, points: number, timeMs: number,
  problems: Record<string, { points: number, wrong: number, pending: number, acceptedAt?: string }>,
}
export type ContestProblemDetail = Omit<PublicProblem, 'timeLimitMs' | 'memoryLimitMb'> & { timeLimitMs: string, memoryLimitMb: string }
export const contestStatus = { scheduled: '開催予定', running: '開催中', ended: '終了' }
export function contestDate(value: string) {
  return new Intl.DateTimeFormat('ja-JP', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
}
