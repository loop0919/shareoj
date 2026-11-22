import { expect, test } from '@playwright/test'
import { refusedUnderLoad, retryRefused } from '../server/utils/busy-retry'

// What ofetch throws for an answer that is not 2xx: the status, and the parsed body in `data`.
const failure = (status: number, data?: unknown) => Object.assign(new Error(String(status)), { response: { status }, data })
// The gateway in front of the API refuses with these bodies; the API's own refusals carry an `error` code.
const gateway503 = () => failure(503, { message: 'Service Unavailable' })
const gateway429 = () => failure(429, { message: 'Too Many Requests' })

// A request that fails with each of `errors` in turn and then answers, and a wait that only counts.
function setup(...errors: unknown[]) {
  let calls = 0
  let waits = 0
  const request = async () => {
    const error = errors[calls++]
    if (error) throw error
    return `answer ${calls}`
  }
  return { run: () => retryRefused(request, async () => { waits++ }), calls: () => calls, waits: () => waits }
}

test('only a gateway refusal counts as refused under load', () => {
  expect(refusedUnderLoad(gateway503())).toBe(true)
  expect(refusedUnderLoad(gateway429())).toBe(true)
  expect(refusedUnderLoad(failure(503))).toBe(true)
  expect(refusedUnderLoad(failure(503, { error: 'database_unavailable' }))).toBe(false)
  expect(refusedUnderLoad(failure(429, { error: 'submission_rate_limited' }))).toBe(false)
  expect(refusedUnderLoad(failure(404, { error: 'not_found' }))).toBe(false)
  expect(refusedUnderLoad(failure(500))).toBe(false)
  expect(refusedUnderLoad(new Error('timeout'))).toBe(false)
})

test('a request the gateway refused is sent once more after a wait', async () => {
  for (const refusal of [gateway503(), gateway429()]) {
    const { run, calls, waits } = setup(refusal)
    expect(await run()).toBe('answer 2')
    expect(calls()).toBe(2)
    expect(waits()).toBe(1)
  }
})

test('a second refusal is passed on without a third try', async () => {
  const second = gateway503()
  const { run, calls, waits } = setup(gateway429(), second)
  await expect(run()).rejects.toBe(second)
  expect(calls()).toBe(2)
  expect(waits()).toBe(1)
})

test('an answer, and a failure that is not a gateway refusal, are not sent again', async () => {
  const fine = setup()
  expect(await fine.run()).toBe('answer 1')
  expect(fine.calls()).toBe(1)
  for (const error of [failure(503, { error: 'database_unavailable' }), failure(429, { error: 'submission_rate_limited' }), failure(404), new Error('timeout')]) {
    const { run, calls, waits } = setup(error)
    await expect(run()).rejects.toBe(error)
    expect(calls()).toBe(1)
    expect(waits()).toBe(0)
  }
})
