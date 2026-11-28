export type ProblemSort = 'newest' | 'oldest' | 'solvers-desc' | 'solvers-asc' | 'favorites-desc' | 'difficulty-desc' | 'difficulty-asc'
export type ProblemSolved = '' | 'unsolved' | 'solved'
export type ProblemContest = '' | 'only' | 'exclude'
export type ProblemConditions = { sort: ProblemSort, solved: ProblemSolved, contest: ProblemContest, min: number, max: number, tag: string }
export type ConditionKey = 'sort' | 'solved' | 'contest' | 'difficulty' | 'tag'
export type ConditionChip = { key: ConditionKey, label: string }

export const problemSorts: readonly { key: ProblemSort, label: string }[] = [
  { key: 'newest', label: '公開日が新しい順' },
  { key: 'oldest', label: '公開日が古い順' },
  { key: 'solvers-desc', label: '正解者数が多い順' },
  { key: 'solvers-asc', label: '正解者数が少ない順' },
  { key: 'favorites-desc', label: 'お気に入りが多い順' },
  { key: 'difficulty-desc', label: '難易度が高い順' },
  { key: 'difficulty-asc', label: '難易度が低い順' },
]
// Without すべて, which the buttons of a status filter have of their own.
export const problemSolvedFilters: readonly { key: Exclude<ProblemSolved, ''>, label: string }[] = [
  { key: 'unsolved', label: '未ACのみ' },
  { key: 'solved', label: 'AC済みのみ' },
]
// Contest problems aim at ranking, the others at the solve itself; a viewer may want only one kind (issue #21).
export const problemContestFilters: readonly { key: ProblemContest, label: string }[] = [
  { key: '', label: 'すべての問題' },
  { key: 'only', label: 'コンテストの問題のみ' },
  { key: 'exclude', label: 'コンテストの問題を除く' },
]

export const defaultConditions: ProblemConditions = { sort: 'newest', solved: '', contest: '', min: 1, max: 10, tag: '' }

const labelOf = <K extends string>(options: readonly { key: K, label: string }[], key: K) => options.find(item => item.key === key)!.label

// A query value counts only when it is a single string; arrays and anything else read as missing.
function single(source: unknown, name: string) {
  if (typeof source !== 'object' || source === null || Array.isArray(source)) return ''
  const value = (source as Record<string, unknown>)[name]
  return typeof value === 'string' ? value : ''
}

// Reads a route query or a remembered cookie. Anything unknown reads as the initial value.
export function parseConditions(query: unknown): ProblemConditions {
  const range = /^([1-9]|10)-([1-9]|10)$/.exec(single(query, 'difficulty'))
  const min = range ? Number(range[1]) : defaultConditions.min
  const max = range ? Number(range[2]) : defaultConditions.max
  const valid = min <= max
  return {
    sort: problemSorts.find(item => item.key === single(query, 'sort'))?.key ?? defaultConditions.sort,
    solved: problemSolvedFilters.find(item => item.key === single(query, 'solved'))?.key ?? defaultConditions.solved,
    contest: problemContestFilters.find(item => item.key === single(query, 'contest'))?.key ?? defaultConditions.contest,
    min: valid ? min : defaultConditions.min,
    max: valid ? max : defaultConditions.max,
    tag: single(query, 'tag'),
  }
}

// The query of a list with these conditions; initial values are left out.
export function conditionQuery(conditions: ProblemConditions): Record<string, string> {
  const query: Record<string, string> = {}
  if (conditions.sort !== defaultConditions.sort) query.sort = conditions.sort
  if (conditions.solved) query.solved = conditions.solved
  if (conditions.contest) query.contest = conditions.contest
  if (conditions.min !== defaultConditions.min || conditions.max !== defaultConditions.max) query.difficulty = `${conditions.min}-${conditions.max}`
  if (conditions.tag) query.tag = conditions.tag
  return query
}

// conditionQuery without the tag: what the cookie keeps.
export function rememberedQuery(conditions: ProblemConditions): Record<string, string> {
  const { tag: _tag, ...query } = conditionQuery(conditions)
  return query
}

// The conditions in force, in the order sort, solved, contest, difficulty, tag. solved is listed only when applied.
export function conditionChips(conditions: ProblemConditions, solvedApplied: boolean): ConditionChip[] {
  const chips: ConditionChip[] = []
  if (conditions.sort !== defaultConditions.sort) chips.push({ key: 'sort', label: labelOf(problemSorts, conditions.sort) })
  if (conditions.solved && solvedApplied) chips.push({ key: 'solved', label: labelOf(problemSolvedFilters, conditions.solved) })
  if (conditions.contest) chips.push({ key: 'contest', label: labelOf(problemContestFilters, conditions.contest) })
  if (conditions.min !== defaultConditions.min || conditions.max !== defaultConditions.max) chips.push({ key: 'difficulty', label: `難易度 Lv.${conditions.min}〜${conditions.max}` })
  if (conditions.tag) chips.push({ key: 'tag', label: `タグ: ${conditions.tag}` })
  return chips
}

// The change that puts one condition back at its initial value, and nothing else.
export function conditionReset(key: ConditionKey): Partial<ProblemConditions> {
  return key === 'difficulty' ? { min: defaultConditions.min, max: defaultConditions.max } : { [key]: defaultConditions[key] }
}
