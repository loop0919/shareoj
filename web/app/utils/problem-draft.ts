import { emptyGenerators, maxFullScore, maxScoringGroups, testFileLimit, testSetLimit, type TestFile, type ProblemDraft, type Scoring } from '~~/shared/types/problem-draft'
// The statement and editorial a new problem starts with, shared with `shareoj init`. The import attribute lets the
// Playwright tests, which load this module in Node, read the JSON too.
import markdownTemplate from '../../../cli/problem-template.json' with { type: 'json' }

export type TestCase = {
  isSample?: boolean
  name?: string
  input: string
  output: string
  inputFile?: TestFile
  outputFile?: TestFile
  // Names of the scoring groups the case belongs to (ADR 0025).
  groups?: string[]
  _inputDirty?: boolean
  _outputDirty?: boolean
}
export function testCaseError(cases: TestCase[]) {
  if (cases.length > 100) return 'テストケースは100件まで登録できます。'
  const encoder = new TextEncoder()
  let total = 0
  const names = new Set<string>()
  for (const [index, item] of cases.entries()) {
    const name = (item.name ?? '').trim()
    if ([...(item.name ?? '')].length > 64 || /[\u0000-\u001f\u007f-\u009f]/.test(item.name ?? '') || (item.name && !name)) return `ケース${index + 1}の名前は64文字以内で、改行や制御文字を含めずに入力してください。`
    if (name && names.has(name)) return `テストケース名「${name}」が重複しています。`
    if (name) names.add(name)
    for (const key of ['input', 'output'] as const) {
      const file = item[`${key}File`]
      const dirty = item[`_${key}Dirty`]
      const value = item[key]
      const size = file && !dirty ? file.size : encoder.encode(value).length
      if (value.includes('\0')) return `ケース${index + 1}に使用できない文字が含まれています。`
      if (size > testFileLimit) return `ケース${index + 1}の入力と出力は、それぞれ16 MiB以内にしてください。`
      total += size
    }
  }
  return total > testSetLimit ? 'テストケース全体を512 MiB以内にしてください。' : ''
}

export function persistedDraft<T extends { testCases: TestCase[] }>(draft: T) {
  return {
    ...draft,
    testCases: draft.testCases.map((item) => {
      const result: Record<string, unknown> = { name: item.name, isSample: item.isSample ?? false }
      // Only partially scored problems list groups, so other drafts keep the shape older servers accept.
      if (item.groups?.length) result.groups = [...item.groups]
      for (const key of ['input', 'output'] as const) {
        const file = item[`${key}File`]
        if (file && !item[`_${key}Dirty`]) {
          result[key] = ''
          result[`${key}File`] = file
        } else {
          result[key] = item[key]
        }
      }
      return result
    }),
  }
}

// What the scoring editor took out of the draft on a mode switch. The page keeps it, so that opening another section
// does not lose it; switching the mode back restores it.
export interface ScoringMemory {
  previous?: { scoring: Scoring, membership: WeakMap<TestCase, string[]> }
  aggregator?: Scoring['aggregator']
}

// The full score of a partially scored problem: the sum of its groups' points (ADR 0025).
export const fullScore = (scoring: Pick<Scoring, 'groups'>) => scoring.groups.reduce((total, group) => total + group.points, 0)

// Why a group name cannot be used, mirroring the server: cases refer to groups by name, so it is never blank and stays
// unique once trimmed.
export function groupNameError(name: string, others: string[]) {
  if (!name.trim() || [...name].length > 64 || /[\u0000-\u001f\u007f-\u009f]/.test(name)) return 'グループ名は64文字以内で、空白だけにせず、改行や制御文字を含めずに入力してください。'
  if (others.some(other => other.trim() === name.trim())) return `グループ名「${name.trim()}」が重複しています。`
  return ''
}

// The structure the server checks at every save (ADR 0025); what is only missing shows in the readiness instead.
export function scoringError(draft: Pick<ProblemDraft, 'scoring'> & { testCases: TestCase[] }) {
  const scoring = draft.scoring
  if (!scoring) return draft.testCases.some(item => item.groups?.length) ? '一括採点の問題では、テストケースをグループに入れられません。' : ''
  if (scoring.groups.length > maxScoringGroups) return `グループは${maxScoringGroups}個まで作れます。`
  for (const [index, group] of scoring.groups.entries()) {
    const error = groupNameError(group.name, scoring.groups.slice(0, index).map(other => other.name))
    if (error) return error
    if (!Number.isInteger(group.points) || group.points < 0 || group.points > maxFullScore) return `「${group.name.trim()}」の配点を0〜1,000,000点の整数にしてください。`
  }
  const total = fullScore(scoring)
  if (scoring.groups.length && (total < 1 || total > maxFullScore)) return '満点（配点の合計）を1〜1,000,000点にしてください。'
  const names = new Set(scoring.groups.map(group => group.name))
  for (const [index, item] of draft.testCases.entries()) {
    if (item.groups?.some((name, at) => !names.has(name) || item.groups!.indexOf(name) !== at)) return `ケース${index + 1}が、存在しないグループに属しています。`
  }
  const code = scoring.aggregator
  if (code && (scoring.mode !== 'aggregate' || new TextEncoder().encode(code.source).length > 65536 || code.source.includes('\0'))) return '集計コードを65,536バイト以内で、NUL文字を含めずに入力してください。'
  return ''
}

export function draftErrors(draft: Pick<ProblemDraft, 'timeLimitMs' | 'memoryLimitMb' | 'checker' | 'interactor' | 'title' | 'markdown'>) {
  const time = Number(draft.timeLimitMs)
  const memory = Number(draft.memoryLimitMb)
  const code = draft.interactor ?? draft.checker
  return {
    checker: draft.checker && draft.interactor ? '判定方法は1つだけ選んでください。' : code && (!code.source.trim() || new TextEncoder().encode(code.source).length > 65536 || code.source.includes('\0')) ? '検証コードを1〜65,536バイトで入力してください。' : '',
    title: draft.title.trim() ? '' : '問題のタイトルを入力してください。',
    markdown: draft.markdown.trim() ? '' : '問題の本文を入力してください。',
    timeLimitMs: Number.isInteger(time) && time >= 100 && time <= 5000 && time % 100 === 0 ? '' : '100〜5,000 ms の範囲で100 ms刻みの値を選んでください。',
    memoryLimitMb: Number.isInteger(memory) && memory >= 64 && memory <= 512 ? '' : '64〜512 MiB の整数を選んでください。',
  }
}

export function exportProblemMarkdown(draft: ProblemDraft): string {
  if (Object.values(draftErrors(draft)).some(Boolean)) throw new Error('Invalid problem draft')
  return [
    '---',
    `title: ${JSON.stringify(draft.title.trim())}`,
    `time_limit_ms: ${Number(draft.timeLimitMs)}`,
    `memory_limit_mb: ${Number(draft.memoryLimitMb)}`,
    '---', '', draft.markdown, '',
  ].join('\n')
}

// A new problem's draft. Each call builds new arrays and generators, so the editor's draft and its reset share nothing.
export function initialProblemDraft() {
  return { checker: null as ProblemDraft['checker'], interactor: null as ProblemDraft['interactor'], scoring: null as ProblemDraft['scoring'], difficulty: null as number | null, title: '', markdown: markdownTemplate.statement, editorial: markdownTemplate.editorial, tags: [] as string[], generators: emptyGenerators(), timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] as TestCase[] }
}
