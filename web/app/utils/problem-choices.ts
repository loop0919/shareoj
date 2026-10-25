import type { AccountSummary } from '~~/shared/types/account-problems'
import { issueMessage } from './problem-readiness'

export interface ProblemChoice {
  id: string
  title: string
  difficulty: number | null
  // Easy or Hard, where the destination cares.
  slot?: string
  // State worth knowing before choosing, such as an existing reservation.
  note?: string
  // Why the problem cannot be chosen; empty when it can.
  reason: string
}

export function problemChoice(problem: AccountSummary, destination: 'publish' | 'featured', note = ''): ProblemChoice {
  const issues = problem.readiness?.[destination] ?? []
  const reason = issues.length ? `${issueMessage(issues[0]!, destination)}${issues.length > 1 ? `（ほか${issues.length - 1}件）` : ''}` : ''
  return { id: problem.id, title: problem.title.trim() || '無題の問題', difficulty: problem.difficulty, note, reason }
}

// Choosable problems first, otherwise in the list's order.
export const choosableFirst = (choices: ProblemChoice[]) => [...choices.filter(choice => !choice.reason), ...choices.filter(choice => choice.reason)]

export const preferenceLabels = { soon: '早めに出したい', later: 'ゆっくりで良い' } as const

// Rounds are always 22:00 in Japan, so the day carries the date.
export const featuredDay = (value: string) => new Intl.DateTimeFormat('ja-JP', { month: 'numeric', day: 'numeric', weekday: 'short', timeZone: 'Asia/Tokyo' }).format(new Date(value))
