import { expect, test } from '@playwright/test'
import { useLibrary } from '../app/composables/useLibrary'

type Call = { url: string, options?: { method?: string, body?: { folderId?: string | null, items?: unknown[] } } }
// useLibrary calls Nuxt's global $fetch; each test answers it in turn.
function answer(...replies: ((call: Call) => unknown)[]) {
  const calls: Call[] = []
  Object.assign(globalThis, {
    $fetch: async (url: string, options?: Call['options']) => {
      const call = { url, options }
      calls.push(call)
      const reply = replies.shift()
      if (!reply) throw new Error(`unexpected ${url}`)
      return reply(call)
    },
  })
  return calls
}
const refused = (code?: string) => () => { throw Object.assign(new Error('refused'), { data: { data: code ? { code } : {} } }) }
const ok = () => undefined

const FOLDER = 'bbbbbbbb-0000-4000-8000-000000000001'
const OTHER = 'bbbbbbbb-0000-4000-8000-000000000002'
const problems = (count: number) => Array.from({ length: count }, (_, i) => ({ kind: 'problem' as const, id: `cccccccc-0000-4000-8000-${String(i).padStart(12, '0')}` }))
const key = (ref: { kind: string, id: string }) => `${ref.kind}:${ref.id}`

test('place sends 100 items at a time', async () => {
  const library = useLibrary()
  const refs = problems(250)
  const calls = answer(ok, ok, ok)
  expect(await library.place(refs, FOLDER)).toBe(true)
  expect(calls.map(call => call.options?.body?.items?.length)).toEqual([100, 100, 50])
  expect(calls.every(call => call.url === '/api/my/folder-items' && call.options?.method === 'PUT' && call.options.body?.folderId === FOLDER)).toBe(true)
  expect(refs.every(ref => library.placed.value.get(key(ref)) === FOLDER)).toBe(true)
  expect(library.message.value).toBe('')
  answer(ok, ok, ok)
  expect(await library.place(refs, null)).toBe(true)
  expect(library.placed.value.size).toBe(0)
})

test('a first batch that fails puts everything back and says nothing changed', async () => {
  const library = useLibrary()
  const refs = problems(150)
  library.placed.value = new Map([[key(refs[0]!), OTHER], [key(refs[120]!), OTHER]])
  const calls = answer(refused())
  expect(await library.place(refs, FOLDER)).toBe(false)
  expect(calls).toHaveLength(1)
  expect([...library.placed.value]).toEqual([[key(refs[0]!), OTHER], [key(refs[120]!), OTHER]])
  expect(library.message.value).toBe('保存できませんでした。もう一度お試しください。何も変わっていません。')
})

test('a later batch that fails keeps what was saved and says how many moved', async () => {
  const library = useLibrary()
  const refs = problems(250)
  library.placed.value = new Map([[key(refs[0]!), OTHER], [key(refs[150]!), OTHER]])
  const calls = answer(ok, refused())
  expect(await library.place(refs, FOLDER)).toBe(false)
  // The third batch is never sent.
  expect(calls).toHaveLength(2)
  // The screen matches the server: the first 100 moved, the rest are where they were.
  expect(refs.slice(0, 100).every(ref => library.placed.value.get(key(ref)) === FOLDER)).toBe(true)
  expect(library.placed.value.get(key(refs[150]!))).toBe(OTHER)
  expect(refs.slice(100).filter(ref => library.placed.value.has(key(ref)))).toEqual([refs[150]])
  expect(library.message.value).toBe('250件中100件を移動しました。残りは移動できませんでした。もう一度お試しください。')
})

test('writes run in turn, so a refused write never undoes a later one', async () => {
  const library = useLibrary()
  const [ref] = problems(1)
  let fail!: () => void
  const failing = new Promise<void>(resolve => { fail = resolve })
  let firstSettled = false
  let sentAfterFirst = false
  const calls = answer(async () => { await failing; refused()() }, () => { sentAfterFirst = firstSettled })
  const first = library.place([ref!], FOLDER)
  void first.then(() => { firstSettled = true })
  const second = library.place([ref!], OTHER)
  await new Promise(resolve => setTimeout(resolve, 10))
  // Only the first is out, and the screen shows only its move.
  expect(calls).toHaveLength(1)
  expect(library.placed.value.get(key(ref!))).toBe(FOLDER)
  fail()
  expect(await first).toBe(false)
  expect(await second).toBe(true)
  expect(calls.map(call => call.options?.body?.folderId)).toEqual([FOLDER, OTHER])
  expect(sentAfterFirst).toBe(true)
  // The first refusal put back only its own move; the second, made after it, stands.
  expect(library.placed.value.get(key(ref!))).toBe(OTHER)
  // Asking for the second cleared the message before the first failed, so the first refusal is still said.
  expect(library.message.value).toBe('保存できませんでした。もう一度お試しください。何も変わっていません。')
  // The next write asked for clears it.
  answer(ok)
  expect(await library.place([ref!], null)).toBe(true)
  expect(library.message.value).toBe('')
  expect(library.placed.value.has(key(ref!))).toBe(false)
})

test('a refused write leaves the loaded list loaded, and a failed load is not a write message', async () => {
  const library = useLibrary()
  // Loading from the start, so the first frame shows the spinner rather than nothing.
  expect(library.loading.value).toBe(true)
  const lists = (call: Call) => call.url === '/api/my/folders' ? { folders: [], items: [] } : { items: [], nextCursor: '' }
  answer(lists, lists, lists, lists)
  await library.load()
  expect([library.loading.value, library.loadError.value]).toEqual([false, ''])
  answer(refused())
  expect(await library.place(problems(1), FOLDER)).toBe(false)
  expect(library.message.value).toBe('保存できませんでした。もう一度お試しください。何も変わっていません。')
  expect(library.loadError.value).toBe('')
  answer(lists, lists, lists, refused())
  await library.load()
  expect(library.loadError.value).toBe('問題と記事を読み込めませんでした。もう一度お試しください。')
  expect(library.message.value).toBe('保存できませんでした。もう一度お試しください。何も変わっていません。')
})

test('a new folder appears once the API has made it', async () => {
  const library = useLibrary()
  let release!: () => void
  const made = new Promise<void>(resolve => { release = resolve })
  answer(async () => { await made; return { id: FOLDER, parentId: null, name: '定期便用' } })
  const creating = library.createFolder('定期便用', null)
  await Promise.resolve()
  expect(library.folders.value).toEqual([])
  release()
  expect(await creating).toEqual({ id: FOLDER, parentId: null, name: '定期便用' })
  expect(library.folders.value).toEqual([{ id: FOLDER, parentId: null, name: '定期便用' }])
  answer(refused('folder_name_taken'))
  expect(await library.createFolder('定期便用', null)).toBeUndefined()
  expect(library.folders.value).toHaveLength(1)
  expect(library.message.value).toBe('同じ場所に同じ名前のフォルダがあります。別の名前にしてください。何も変わっていません。')
})

test('a refusal the caller shows itself does not reach message', async () => {
  const library = useLibrary()
  const heard: string[] = []
  const taken = '同じ場所に同じ名前のフォルダがあります。別の名前にしてください。何も変わっていません。'
  answer(refused('folder_name_taken'))
  expect(await library.createFolder('定期便用', null, text => heard.push(text))).toBeUndefined()
  library.folders.value = [{ id: FOLDER, parentId: null, name: '定期便' }]
  answer(refused('folder_name_taken'))
  expect(await library.updateFolder(FOLDER, 'メモ', null, text => heard.push(text))).toBe(false)
  expect(library.folders.value).toEqual([{ id: FOLDER, parentId: null, name: '定期便' }])
  expect(heard).toEqual([taken, taken])
  expect(library.message.value).toBe('')
})
