import { test, expect } from '@playwright/test'
import { conditionChips, conditionQuery, conditionReset, defaultConditions, parseConditions, rememberedQuery } from '../app/utils/problem-conditions'

test('conditions read from a query and write back without initial values', () => {
  expect(parseConditions({})).toEqual(defaultConditions)
  const all = parseConditions({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP', page: '2' })
  expect(all).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', min: 3, max: 6, tag: 'DP' })
  expect(conditionQuery(all)).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP' })
  expect(rememberedQuery(all)).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6' })
  expect(conditionQuery(defaultConditions)).toEqual({})
})

test('unknown values read as the initial ones', () => {
  for (const bad of [{ sort: 'popular' }, { sort: ['oldest', 'newest'] }, { solved: 'yes' }, { contest: 'yes' }, { difficulty: '6-3' }, { difficulty: '0-3' }, { difficulty: '3-11' }, { difficulty: '3' }, { difficulty: '1-10' }, { tag: ['a', 'b'] }])
    expect(parseConditions(bad)).toEqual(defaultConditions)
  for (const broken of [null, undefined, 'sort=oldest', 42, [], { sort: { key: 'oldest' } }])
    expect(parseConditions(broken)).toEqual(defaultConditions)
})

test('chips name the conditions in force', () => {
  const all = parseConditions({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP' })
  expect(conditionChips(all, true).map(chip => chip.label)).toEqual(['正解者数が多い順', '未ACのみ', 'コンテストの問題のみ', '難易度 Lv.3〜6', 'タグ: DP'])
  expect(conditionChips(all, false).map(chip => chip.key)).toEqual(['sort', 'contest', 'difficulty', 'tag'])
  expect(conditionChips(parseConditions({ solved: 'solved', contest: 'exclude' }), true).map(chip => chip.label)).toEqual(['AC済みのみ', 'コンテストの問題を除く'])
  expect(conditionChips(defaultConditions, true)).toEqual([])
  expect(conditionChips(parseConditions({ difficulty: '4-4' }), true)[0].label).toBe('難易度 Lv.4〜4')
  expect(conditionQuery({ ...all, ...conditionReset('difficulty') })).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', tag: 'DP' })
  expect(conditionReset('contest')).toEqual({ contest: '' })
})
