import assert from 'node:assert/strict'
import { test } from 'node:test'
import { durationsOf, line, split, testsOf } from './shard-tests.mjs'

// The shape of `playwright test --list --reporter=json`, and of the JSON report with results.
const report = {
  config: { configFile: '/repo/web/playwright.config.ts' },
  suites: [
    { file: 'a.spec.ts', specs: [{ title: 'first', tests: [{ results: [{ duration: 4000 }] }] }], suites: [] },
    {
      file: 'b.spec.ts',
      specs: [{ title: 'top', tests: [{ results: [{ duration: 1000 }, { duration: 3000 }] }] }],
      suites: [{ title: 'group', specs: [{ title: 'inner', tests: [{ results: [] }] }], suites: [{ title: 'deeper', specs: [{ title: 'last', tests: [{ results: [{ duration: 500 }] }] }] }] }],
    },
  ],
}

test('lists every test with its describe titles, in the order Playwright runs them', () => {
  assert.deepEqual(testsOf(report).map(line), ['a.spec.ts › first', 'b.spec.ts › top', 'b.spec.ts › group › inner', 'b.spec.ts › group › deeper › last'])
})

test('refuses a title that --test-list would split', () => {
  const broken = { suites: [{ file: 'a.spec.ts', specs: [{ title: 'a › b', tests: [{}] }], suites: [] }] }
  assert.throws(() => testsOf(broken), /›/)
})

test('records the last attempt of each test, in seconds, and skips tests that did not run', () => {
  assert.deepEqual(durationsOf(report), { 'a.spec.ts › first': 4, 'b.spec.ts › top': 3, 'b.spec.ts › group › deeper › last': 0.5 })
})

test('puts every test in exactly one shard, keeps the running order, and evens out the time', () => {
  const tests = ['a', 'b', 'c', 'd', 'e', 'f'].map(title => ({ file: 'x.spec.ts', titles: [title] }))
  const durations = { 'x.spec.ts › a': 6, 'x.spec.ts › b': 1, 'x.spec.ts › c': 5, 'x.spec.ts › d': 2, 'x.spec.ts › e': 4, 'x.spec.ts › f': 3 }
  const shards = split(tests, durations, 3)
  assert.deepEqual(shards.flat().map(line).sort(), tests.map(line).sort())
  for (const shard of shards) assert.deepEqual(shard, tests.filter(item => shard.includes(item)))
  assert.deepEqual(shards.map(shard => shard.reduce((sum, item) => sum + durations[line(item)], 0)), [7, 7, 7])
})

test('counts a test without a recorded duration as the median of the others', () => {
  const tests = ['old1', 'old2', 'old3', 'new1', 'new2'].map(title => ({ file: 'x.spec.ts', titles: [title] }))
  const durations = { 'x.spec.ts › old1': 1, 'x.spec.ts › old2': 2, 'x.spec.ts › old3': 9 }
  const shards = split(tests, durations, 2).map(shard => shard.map(item => item.titles[0]))
  assert.deepEqual(shards, [['old3'], ['old1', 'old2', 'new1', 'new2']])
})

test('spreads tests recorded at zero seconds instead of stacking them on one shard', () => {
  const tests = ['a', 'b', 'c', 'd'].map(title => ({ file: 'x.spec.ts', titles: [title] }))
  const durations = { 'x.spec.ts › a': 0, 'x.spec.ts › b': 0, 'x.spec.ts › c': 0, 'x.spec.ts › d': 0 }
  assert.deepEqual(split(tests, durations, 4).map(shard => shard.length), [1, 1, 1, 1])
})

test('splits the same way every time', () => {
  const tests = Array.from({ length: 20 }, (_, i) => ({ file: `${i % 3}.spec.ts`, titles: [`t${i}`] }))
  assert.deepEqual(split(tests, {}, 4), split(tests, {}, 4))
  assert.equal(split(tests, {}, 4).flat().length, 20)
})
