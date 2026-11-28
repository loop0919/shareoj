import { isLanguageKey, isRuntimeId, isSubmissionUser, submissionSortKeys, submissionVerdicts } from '~~/shared/utils/submission-conditions'
import type { SubmissionSort } from '~~/shared/utils/submission-conditions'

export { submissionVerdicts }
export type { SubmissionSort }
export type RuntimeLanguage = { key: string, label: string, runtimes: { id: string, label: string }[] }
export type SubmissionConditions = { sort: SubmissionSort, verdict: string, language: string, runtime: string, user: string, problem: string }
export type SubmissionConditionKey = 'sort' | 'verdict' | 'language' | 'user' | 'problem'

const sortLabels: Record<SubmissionSort, string> = {
  newest: '提出日時が新しい順',
  oldest: '提出日時が古い順',
  shortest: 'コード長が短い順',
  fastest: '実行時間が短い順',
}
export const submissionSorts: readonly { key: SubmissionSort, label: string }[] = submissionSortKeys.map(key => ({ key, label: sortLabels[key] }))

export const defaultSubmissionConditions: SubmissionConditions = { sort: 'newest', verdict: '', language: '', runtime: '', user: '', problem: '' }

const problemMax = 100

// A query value counts only when it is a single string; arrays and anything else read as missing.
function single(source: unknown, name: string) {
  if (typeof source !== 'object' || source === null || Array.isArray(source)) return ''
  const value = (source as Record<string, unknown>)[name]
  return typeof value === 'string' ? value : ''
}

// Reads a route query. Unknown values read as the initial ones; with both language and runtime, runtime wins.
// problem is kept as written (a contest's letter); the page resolves it.
export function parseSubmissionConditions(query: unknown): SubmissionConditions {
  const sort = single(query, 'sort')
  const verdict = single(query, 'verdict')
  const language = single(query, 'language')
  const runtime = single(query, 'runtime')
  const user = single(query, 'user')
  const problem = single(query, 'problem')
  const runtimeValid = isRuntimeId(runtime)
  return {
    sort: submissionSortKeys.find(key => key === sort) ?? defaultSubmissionConditions.sort,
    verdict: submissionVerdicts.includes(verdict) ? verdict : '',
    language: !runtimeValid && isLanguageKey(language) ? language : '',
    runtime: runtimeValid ? runtime : '',
    user: isSubmissionUser(user) ? user : '',
    problem: problem.length <= problemMax ? problem : '',
  }
}

// The API refuses a language or a runtime its catalogue does not list, which the shape alone cannot tell. So one that
// languages (the judge's catalogue) does not list reads as すべて, as does any while the catalogue is not known
// (undefined).
export function listedLanguage(conditions: SubmissionConditions, languages: RuntimeLanguage[] | undefined): SubmissionConditions {
  const { language: key, runtime: id } = conditions
  if (!key && !id) return conditions
  const listed = languages?.some(language => id ? language.runtimes.some(runtime => runtime.id === id) : language.key === key)
  return listed ? conditions : { ...conditions, language: '', runtime: '' }
}

// The query of a list with these conditions; initial values are left out.
export function submissionConditionQuery(conditions: SubmissionConditions): Record<string, string> {
  const query: Record<string, string> = {}
  if (conditions.sort !== defaultSubmissionConditions.sort) query.sort = conditions.sort
  if (conditions.verdict) query.verdict = conditions.verdict
  if (conditions.language) query.language = conditions.language
  if (conditions.runtime) query.runtime = conditions.runtime
  if (conditions.user) query.user = conditions.user
  if (conditions.problem) query.problem = conditions.problem
  return query
}

// The options of the 言語 select: a group per language, with the whole language first when it has two runtimes or more.
export function languageOptions(languages: RuntimeLanguage[]): { label: string, options: { value: string, label: string }[] }[] {
  return languages.map(language => ({
    label: language.label,
    options: [
      ...(language.runtimes.length >= 2 ? [{ value: `language:${language.key}`, label: `${language.label}（すべて）` }] : []),
      ...language.runtimes.map(runtime => ({ value: `runtime:${runtime.id}`, label: runtime.label })),
    ],
  }))
}

// value is 'language:cpp' or 'runtime:cpp23-gcc'; '' is すべて.
export function languageValue(conditions: SubmissionConditions): string {
  if (conditions.runtime) return `runtime:${conditions.runtime}`
  return conditions.language ? `language:${conditions.language}` : ''
}

export function languagePatch(value: string): Pick<SubmissionConditions, 'language' | 'runtime'> {
  // A value of the wrong shape reads as すべて, as parseSubmissionConditions would on reload.
  if (value.startsWith('language:')) {
    const language = value.slice('language:'.length)
    if (isLanguageKey(language)) return { language, runtime: '' }
  }
  else if (value.startsWith('runtime:')) {
    const runtime = value.slice('runtime:'.length)
    if (isRuntimeId(runtime)) return { language: '', runtime }
  }
  return { language: '', runtime: '' }
}

// The conditions in force, in the order sort, verdict, language, user, problem.
export function submissionConditionChips(conditions: SubmissionConditions, languages: RuntimeLanguage[]): { key: SubmissionConditionKey, label: string }[] {
  const chips: { key: SubmissionConditionKey, label: string }[] = []
  if (conditions.sort !== defaultSubmissionConditions.sort) chips.push({ key: 'sort', label: sortLabels[conditions.sort] })
  if (conditions.verdict) chips.push({ key: 'verdict', label: `結果: ${conditions.verdict}` })
  if (conditions.runtime) {
    const label = languages.flatMap(language => language.runtimes).find(runtime => runtime.id === conditions.runtime)?.label
    chips.push({ key: 'language', label: `言語: ${label ?? conditions.runtime}` })
  }
  else if (conditions.language) {
    chips.push({ key: 'language', label: `言語: ${languages.find(language => language.key === conditions.language)?.label ?? conditions.language}` })
  }
  if (conditions.user) chips.push({ key: 'user', label: `提出者: ${conditions.user}` })
  if (conditions.problem) chips.push({ key: 'problem', label: `問題: ${conditions.problem}` })
  return chips
}

// The patch that puts one condition back to its initial value ('language' resets language and runtime).
export function submissionConditionReset(key: SubmissionConditionKey): Partial<SubmissionConditions> {
  return key === 'language' ? { language: '', runtime: '' } : { [key]: defaultSubmissionConditions[key] }
}
