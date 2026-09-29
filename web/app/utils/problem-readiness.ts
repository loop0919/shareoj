import type { Readiness } from '~~/shared/types/account-problems'

export type Destination = keyof Readiness
export type EditorSection = 'statement' | 'tests' | 'checker' | 'editorial'
export interface ProblemStatusInput { readiness?: Readiness, publishedVersion: number, contestScheduled: boolean, featuredPreference: string }

export const destinationNames: Record<Destination, string> = { publish: '公開', contest: 'コンテスト', featured: '定期便' }

const messages: Record<string, string> = {
  invalid_draft: '保存済みの内容に、現在の規則に合わない項目があります。編集画面で見直して保存し直してください',
  title_missing: 'タイトルがありません',
  statement_missing: '問題文がありません',
  checker_source_missing: '検証コードが空です',
  checker_runtime_unavailable: '検証コードの言語が現在使えません（ジャッジの保守中の可能性があります）',
  interactor_source_missing: '対話用ジャッジのコードが空です',
  interactor_runtime_unavailable: '対話用ジャッジの言語が現在使えません（ジャッジの保守中の可能性があります）',
  test_cases_missing: 'テストケースが1件もありません',
  difficulty_missing: '難易度が設定されていません',
  editorial_missing: '解説がありません',
  published: '公開中の問題はコンテストに登録できません',
  ever_published: '一度公開した問題は定期便に応募できません',
  featured_limit: '定期便に応募できるのは作者ごとに3件までです',
}
// The same state reads differently per destination.
const destinationMessages: Partial<Record<Destination, Record<string, string>>> = {
  publish: { in_contest: 'コンテストに登録済みのため、コンテストが終わるまで公開できません' },
  contest: { in_contest: 'コンテストに登録済みです（登録できるのは1つのコンテストだけです）' },
  featured: { in_contest: 'コンテストに登録した問題は応募できません' },
}
export const issueMessage = (code: string, destination?: Destination) =>
  (destination && destinationMessages[destination]?.[code]) ?? messages[code] ?? `確認が必要な項目があります（${code}）`

const sections: Record<string, EditorSection> = {
  title_missing: 'statement', statement_missing: 'statement', difficulty_missing: 'statement',
  test_cases_missing: 'tests',
  checker_source_missing: 'checker', checker_runtime_unavailable: 'checker', interactor_source_missing: 'checker', interactor_runtime_unavailable: 'checker',
  editorial_missing: 'editorial',
}
export const issueSection = (code: string): EditorSection | undefined => sections[code]

// Reasons about the problem itself; the rest describe where it already is.
const stateIssues = new Set(['published', 'ever_published', 'in_contest', 'featured_limit'])

// Contests need a subset of 定期便's content, so the union is what "complete" means.
export function contentIssues(readiness?: Readiness): string[] {
  if (!readiness) return []
  return [...new Set([...readiness.contest, ...readiness.featured])].filter(code => !stateIssues.has(code))
}

export interface ProblemStatus {
  kind: 'published' | 'contest' | 'featured' | 'ready' | 'incomplete' | 'unknown'
  label: string
  summary: string
  issues: string[]
}

// Five states only: ×準備中, ✅準備中, 応募中（コンテスト）, 応募中（定期便）, 公開中.
export function problemStatus(problem: ProblemStatusInput): ProblemStatus {
  if (problem.publishedVersion > 0) return { kind: 'published', label: '公開中', summary: '公開中です', issues: [] }
  if (problem.contestScheduled) return { kind: 'contest', label: '応募中（コンテスト）', summary: 'コンテストに登録済みです', issues: [] }
  if (problem.featuredPreference) {
    // An application that stopped qualifying is skipped at selection; say so instead of waiting silently.
    const issues = problem.readiness?.featured ?? []
    return { kind: 'featured', label: '応募中（定期便）', summary: issues.length ? '定期便に応募中ですが、このままでは選出されません' : '定期便に応募中です', issues }
  }
  // Tester views and older API responses carry no readiness; do not guess.
  if (!problem.readiness) return { kind: 'unknown', label: '準備中', summary: '準備中です', issues: [] }
  const issues = contentIssues(problem.readiness)
  return issues.length
    ? { kind: 'incomplete', label: '準備中', summary: '準備中です。足りない項目があります', issues }
    : { kind: 'ready', label: '準備中', summary: '準備中です。コンテストと定期便に出せます', issues: [] }
}
