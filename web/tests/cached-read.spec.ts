import { expect, test } from '@playwright/test'
import { cachedRead } from '../server/utils/cached-read'

// A cache with a clock the test moves and a source the test answers.
function setup() {
  let time = 1_000_000
  let reads = 0
  let next: () => Promise<string> = async () => `value ${reads}`
  const cache = cachedRead<string>(15_000, 300_000, () => time)
  const get = () => cache(() => { reads++; return next() })
  return { get, reads: () => reads, pass: (ms: number) => { time += ms }, answer: (read: () => Promise<string>) => { next = read } }
}

test('readers within the fresh time share one read, and the next one after it reads again', async () => {
  const { get, reads, pass } = setup()
  expect(await get()).toBe('value 1')
  pass(14_999)
  expect(await get()).toBe('value 1')
  expect(reads()).toBe(1)
  pass(1)
  expect(await get()).toBe('value 2')
  expect(reads()).toBe(2)
})

test('readers that arrive while a read is under way wait for it instead of starting their own', async () => {
  const { get, reads, answer } = setup()
  let finish!: (value: string) => void
  answer(() => new Promise(resolve => { finish = resolve }))
  const both = Promise.all([get(), get()])
  finish('shared')
  expect(await both).toEqual(['shared', 'shared'])
  expect(reads()).toBe(1)
})

test('a failed read answers the last value, until that value is too old', async () => {
  const { get, reads, pass, answer } = setup()
  expect(await get()).toBe('value 1')
  answer(() => Promise.reject(new Error('503')))
  pass(15_000)
  expect(await get()).toBe('value 1')
  // The old value is not fresh again: the next reader tries the source once more.
  expect(await get()).toBe('value 1')
  expect(reads()).toBe(3)
  pass(285_000)
  await expect(get()).rejects.toThrow('503')
})

test('a failed first read is passed on and not kept', async () => {
  const { get, reads, answer } = setup()
  answer(() => Promise.reject(new Error('503')))
  await expect(get()).rejects.toThrow('503')
  answer(async () => 'recovered')
  expect(await get()).toBe('recovered')
  expect(reads()).toBe(2)
})

test('the runtime catalog is read from the API once for many requests', async ({ request }) => {
  const reads = async () => (await (await request.get('http://127.0.0.1:18080/fixture/runtime-reads')).json()).reads as number
  // The first request fills the cache if no other test has; the count starts after it.
  expect((await request.get('/api/runtimes')).status()).toBe(200)
  const before = await reads()
  for (let i = 0; i < 10; i++) {
    const response = await request.get('/api/runtimes')
    expect(response.status()).toBe(200)
    expect(response.headers()['cache-control']).toBe('no-store')
    expect((await response.json()).items[0]).toEqual({ id: 'cpp23-gcc', label: 'C++23 (GCC)' })
  }
  // Ten requests take far less than the fresh time, so at most one of them falls after it ends.
  expect(await reads() - before).toBeLessThanOrEqual(1)
})
