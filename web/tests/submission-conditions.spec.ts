import { test, expect } from '@playwright/test'
import {
  defaultSubmissionConditions, languageOptions, languagePatch, languageValue, parseSubmissionConditions,
  submissionConditionChips, submissionConditionQuery, submissionConditionReset,
} from '../app/utils/submission-conditions'

const languages = [
  { key: 'cpp', label: 'C++', runtimes: [{ id: 'cpp17', label: 'C++17 (GCC)' }, { id: 'cpp23-gcc', label: 'C++23 (GCC)' }] },
  { key: 'rust', label: 'Rust', runtimes: [{ id: 'rust2024', label: 'Rust (Edition 2024)' }] },
]

test('conditions read from a query and write back without initial values', () => {
  expect(parseSubmissionConditions({})).toEqual(defaultSubmissionConditions)
  const all = parseSubmissionConditions({ sort: 'fastest', verdict: 'AC', language: 'cpp', user: 'alice', problem: 'A', view: 'submissions', page: '2' })
  expect(all).toEqual({ sort: 'fastest', verdict: 'AC', language: 'cpp', runtime: '', user: 'alice', problem: 'A' })
  expect(submissionConditionQuery(all)).toEqual({ sort: 'fastest', verdict: 'AC', language: 'cpp', user: 'alice', problem: 'A' })
  expect(submissionConditionQuery(defaultSubmissionConditions)).toEqual({})
  expect(parseSubmissionConditions({ language: 'cpp', runtime: 'cpp17' })).toMatchObject({ language: '', runtime: 'cpp17' })
  expect(submissionConditionQuery(parseSubmissionConditions({ runtime: 'cpp17' }))).toEqual({ runtime: 'cpp17' })
})

test('unknown values read as the initial ones', () => {
  for (const bad of [{ sort: 'longest' }, { sort: ['fastest', 'newest'] }, { verdict: 'ac' }, { verdict: 'OK' }, { user: ['a', 'b'] }, { user: 'a b' }, { language: 'C++' }, { runtime: 'cpp23-gcc-isolate' }, { runtime: 'cpp17-local' }, { problem: ['A'] }, { problem: 'x'.repeat(101) }])
    expect(parseSubmissionConditions(bad)).toEqual(defaultSubmissionConditions)
  for (const broken of [null, undefined, 'sort=fastest', 42, []]) expect(parseSubmissionConditions(broken)).toEqual(defaultSubmissionConditions)
  // An invalid runtime does not hide a valid language.
  expect(parseSubmissionConditions({ language: 'cpp', runtime: 'cpp17-isolate' })).toMatchObject({ language: 'cpp', runtime: '' })
})

test('the language select groups runtimes and offers the whole language', () => {
  expect(languageOptions(languages)).toEqual([
    { label: 'C++', options: [{ value: 'language:cpp', label: 'C++（すべて）' }, { value: 'runtime:cpp17', label: 'C++17 (GCC)' }, { value: 'runtime:cpp23-gcc', label: 'C++23 (GCC)' }] },
    { label: 'Rust', options: [{ value: 'runtime:rust2024', label: 'Rust (Edition 2024)' }] },
  ])
  expect(languageValue(parseSubmissionConditions({ language: 'cpp' }))).toBe('language:cpp')
  expect(languageValue(parseSubmissionConditions({ runtime: 'cpp17' }))).toBe('runtime:cpp17')
  expect(languageValue(defaultSubmissionConditions)).toBe('')
  expect(languagePatch('language:cpp')).toEqual({ language: 'cpp', runtime: '' })
  expect(languagePatch('runtime:cpp17')).toEqual({ language: '', runtime: 'cpp17' })
  expect(languagePatch('')).toEqual({ language: '', runtime: '' })
})

test('chips name the conditions in force', () => {
  const all = parseSubmissionConditions({ sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc', user: 'alice', problem: 'A' })
  expect(submissionConditionChips(all, languages).map(chip => chip.label)).toEqual(['コード長が短い順', '結果: AC', '言語: C++23 (GCC)', '提出者: alice', '問題: A'])
  expect(submissionConditionChips(all, languages).map(chip => chip.key)).toEqual(['sort', 'verdict', 'language', 'user', 'problem'])
  expect(submissionConditionChips(parseSubmissionConditions({ language: 'cpp' }), languages)[0].label).toBe('言語: C++')
  // A language the catalogue does not know yet (the list has not loaded) still shows a chip, by its key.
  expect(submissionConditionChips(parseSubmissionConditions({ language: 'cpp' }), [])[0].label).toBe('言語: cpp')
  expect(submissionConditionChips(defaultSubmissionConditions, languages)).toEqual([])
  expect(submissionConditionReset('language')).toEqual({ language: '', runtime: '' })
  expect(submissionConditionReset('sort')).toEqual({ sort: 'newest' })
  expect(submissionConditionQuery({ ...all, ...submissionConditionReset('language') })).toEqual({ sort: 'shortest', verdict: 'AC', user: 'alice', problem: 'A' })
})
