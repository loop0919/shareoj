import { expect, test } from '@playwright/test'

const session = { cookie: 'openoj_access=valid-access' }

test('the list route passes the order and the difficulty range on', async ({ request }) => {
  const oldest = await (await request.get('/api/problems?tag=pagination&sort=oldest&limit=20')).json()
  expect(oldest.items[0].id).toBe('00000000-0000-4000-8000-000000000120')
  expect(oldest.solved).toBe('')
  expect((await (await request.get('/api/problems?tag=pagination&difficulty=1-9')).json()).total).toBe(0)
  expect((await (await request.get('/api/problems?tag=pagination&difficulty=10-10')).json()).total).toBe(120)
  expect((await request.get('/api/problems?sort=popular')).status()).toBe(400)
  expect((await request.get('/api/problems?difficulty=7-3')).status()).toBe(400)
})

test('the solved filter needs a session and says whether it was applied', async ({ request }) => {
  const mine = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: session })).json()
  expect([mine.total, mine.solved]).toEqual([60, 'unsolved'])
  const done = await (await request.get('/api/problems?tag=pagination&solved=solved&sort=oldest', { headers: session })).json()
  expect([done.total, done.solved, done.items[0].id]).toEqual([60, 'solved', '00000000-0000-4000-8000-000000000119'])
  const guest = await (await request.get('/api/problems?tag=pagination&solved=unsolved')).json()
  expect([guest.total, guest.solved]).toEqual([120, ''])
  // A session the API refuses falls back to the whole list.
  const expired = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: { cookie: 'openoj_access=expired' } })).json()
  expect([expired.total, expired.solved]).toEqual([120, ''])
  // An unknown value is not sent on.
  const unknown = await (await request.get('/api/problems?tag=pagination&solved=maybe', { headers: session })).json()
  expect([unknown.total, unknown.solved]).toEqual([120, ''])
})

test('a refused order stays a 400 on the signed-in path', async ({ request }) => {
  expect((await request.get('/api/problems?sort=popular&solved=unsolved', { headers: session })).status()).toBe(400)
})

test('the untagged problem has no difficulty and is not solved', async ({ request }) => {
  expect((await (await request.get('/api/problems?difficulty=1-10', { headers: session })).json()).total).toBe(0)
  const unsolved = await (await request.get('/api/problems?solved=unsolved', { headers: session })).json()
  expect([unsolved.total, unsolved.solved]).toEqual([1, 'unsolved'])
  expect((await (await request.get('/api/problems?solved=solved', { headers: session })).json()).total).toBe(0)
})
