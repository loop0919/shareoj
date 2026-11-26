import { expect, test } from '@playwright/test'
import { placeNewItem, placementTimeoutMs } from '../app/utils/folder-items'

const FOLDER = 'aaaaaaaa-0000-4000-8000-000000000001'
const OTHER = 'aaaaaaaa-0000-4000-8000-000000000002'
const item = { kind: 'post' as const, id: 'cccccccc-0000-4000-8000-000000000001' }

// placeNewItem calls Nuxt's global $fetch; this one records each call and refuses when told to.
function answer(refuse: () => boolean) {
  const sent: { url: string, options: unknown }[] = []
  Object.assign(globalThis, {
    $fetch: async (url: string, options: unknown) => {
      sent.push({ url, options })
      if (refuse()) throw new Error('refused')
    },
  })
  return sent
}

test('a new item goes into the folder its editor was opened in, and a failure is dropped', async () => {
  let refuse = false
  const sent = answer(() => refuse)
  // Anything but a UUID, such as 未整理 or a repeated query, names no folder.
  for (const folder of [undefined, '', 'unsorted', [FOLDER], `${FOLDER}x`]) await placeNewItem(folder, item)
  expect(sent).toEqual([])
  await placeNewItem(FOLDER, item)
  // The save waits for the placement, so a slow one is given up after five seconds.
  expect(placementTimeoutMs).toBe(5000)
  expect(sent).toEqual([{ url: '/api/my/folder-items', options: { method: 'PUT', body: { folderId: FOLDER, items: [item] }, timeout: 5000 } }])
  refuse = true
  await expect(placeNewItem(OTHER, item)).resolves.toBeUndefined()
  expect(sent).toHaveLength(2)
})
