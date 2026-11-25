import { expect, test } from '@playwright/test'
import { accountListSchema } from '../shared/types/account-problems'
import { postSummarySchema } from '../shared/types/post'
import { canNest, descendantIds, folderCounts, folderPath, placeNewItem, sortedChildren } from '../app/utils/folder-tree'
import { libraryEntries } from '../app/utils/library-entries'

const A = 'aaaaaaaa-0000-4000-8000-000000000001'
const B = 'aaaaaaaa-0000-4000-8000-000000000002'
const C = 'aaaaaaaa-0000-4000-8000-000000000003'
const D = 'aaaaaaaa-0000-4000-8000-000000000004'
const E = 'aaaaaaaa-0000-4000-8000-000000000005'
const MISSING = 'aaaaaaaa-0000-4000-8000-0000000000ff'
const f = (id: string, parentId: string | null, name: string) => ({ id, parentId, name })
// A holds B, which holds C, three levels deep; D holds E, two levels.
const tree = [f(A, null, 'コンテスト用'), f(B, A, '第3回'), f(C, B, '予備'), f(D, null, 'あとで'), f(E, D, '候補')]

test('children are sorted by name in Japanese order, and a path runs from the top', () => {
  expect(sortedChildren(tree, null).map(x => x.name)).toEqual(['あとで', 'コンテスト用'])
  expect(sortedChildren(tree, A).map(x => x.name)).toEqual(['第3回'])
  expect(sortedChildren(tree, C)).toEqual([])
  expect(folderPath(tree, C).map(x => x.name)).toEqual(['コンテスト用', '第3回', '予備'])
  expect(folderPath(tree, MISSING)).toEqual([])
  expect(descendantIds(tree, A).sort()).toEqual([B, C].sort())
  expect(descendantIds(tree, C)).toEqual([])
})

test('a folder cannot go into itself, below itself, or deeper than three levels', () => {
  expect(canNest(tree, null, C)).toBe(false) // a fourth level
  expect(canNest(tree, null, B)).toBe(true)
  expect(canNest(tree, A, C)).toBe(false) // below itself
  expect(canNest(tree, A, A)).toBe(false)
  expect(canNest(tree, D, B)).toBe(false) // D and E take two levels, and below B they would start at the third
  expect(canNest(tree, D, A)).toBe(true)
  expect(canNest(tree, B, null)).toBe(true)
  expect(canNest(tree, null, MISSING)).toBe(false)
})

test('a folder counts what its lower folders hold, and a placement in an unknown folder counts as unsorted', () => {
  const placed = new Map([['problem:p1', B], ['problem:p2', C], ['post:x1', A], ['problem:p3', MISSING]])
  const counts = folderCounts(tree, placed, ['problem:p1', 'problem:p2', 'post:x1', 'problem:p3', 'problem:p4'])
  expect(counts.all).toBe(5)
  expect(counts.unsorted).toBe(2)
  expect(Object.fromEntries(counts.byFolder)).toEqual({ [A]: 3, [B]: 2, [C]: 1, [D]: 0, [E]: 0 })
})

test('problems and posts make one list, newest first, with their kind and status', () => {
  const problems = (items: object[]) => accountListSchema.parse({ items, nextCursor: '' }).items
  const own = problems([{ id: 'bbbbbbbb-0000-4000-8000-000000000001', title: '自分の問題', scheduledAt: '2026-12-01T00:00:00Z', updatedAt: '2026-11-20T08:00:00+09:00' }])
  const testing = problems([{ id: 'bbbbbbbb-0000-4000-8000-000000000002', title: 'テスト中の問題', author: 'bob', publishedVersion: 2, updatedAt: '2026-11-19T23:30:00Z' }])
  const posts = [
    { id: 'cccccccc-0000-4000-8000-000000000002', title: '公開中の記事', publishedVersion: 1, publishedAt: '2026-11-10T00:00:00Z', updatedAt: '2026-11-10T00:00:00Z', isOperator: false },
    { id: 'cccccccc-0000-4000-8000-000000000001', title: '下書きの記事', publishedVersion: 0, publishedAt: null, updatedAt: '2026-11-10T00:00:00Z', isOperator: false },
  ].map(post => postSummarySchema.parse(post))
  const entries = libraryEntries(own, testing, posts)
  // The time is compared, not the text: 08:00 in Japan on the 20th is before 23:30 UTC on the 19th.
  // Two posts saved at the same time follow their keys.
  expect(entries.map(({ key, kind, status, title }) => ({ key, kind, status, title }))).toEqual([
    { key: 'problem:bbbbbbbb-0000-4000-8000-000000000002', kind: 'testing', status: 'published', title: 'テスト中の問題' },
    { key: 'problem:bbbbbbbb-0000-4000-8000-000000000001', kind: 'own', status: 'scheduled', title: '自分の問題' },
    { key: 'post:cccccccc-0000-4000-8000-000000000001', kind: 'post', status: 'preparing', title: '下書きの記事' },
    { key: 'post:cccccccc-0000-4000-8000-000000000002', kind: 'post', status: 'published', title: '公開中の記事' },
  ])
  expect(entries.map(entry => entry.ref)).toEqual([
    { kind: 'problem', id: 'bbbbbbbb-0000-4000-8000-000000000002' },
    { kind: 'problem', id: 'bbbbbbbb-0000-4000-8000-000000000001' },
    { kind: 'post', id: 'cccccccc-0000-4000-8000-000000000001' },
    { kind: 'post', id: 'cccccccc-0000-4000-8000-000000000002' },
  ])
  expect(entries[0]!.problem).toBe(testing[0])
  expect(entries[3]!.post).toBe(posts[0])
})

test('a new item goes into the folder its editor was opened in, and a failure is dropped', async () => {
  const sent: { url: string, options: unknown }[] = []
  let refuse = false
  Object.assign(globalThis, {
    $fetch: async (url: string, options: unknown) => {
      sent.push({ url, options })
      if (refuse) throw new Error('refused')
    },
  })
  const item = { kind: 'post' as const, id: 'cccccccc-0000-4000-8000-000000000001' }
  // Anything but a UUID, such as 未整理 or a repeated query, names no folder.
  for (const folder of [undefined, '', 'unsorted', [A], `${A}x`]) await placeNewItem(folder, item)
  expect(sent).toEqual([])
  await placeNewItem(A, item)
  expect(sent).toEqual([{ url: '/api/my/folder-items', options: { method: 'PUT', body: { folderId: A, items: [item] } } }])
  refuse = true
  await expect(placeNewItem(B, item)).resolves.toBeUndefined()
  expect(sent).toHaveLength(2)
})
