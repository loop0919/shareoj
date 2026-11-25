import { ref } from 'vue'
import { accountListSchema } from '~~/shared/types/account-problems'
import { folderSchema, folderTreeSchema, maxFolderItems, type Folder, type FolderRef } from '~~/shared/types/folders'
import { postListSchema } from '~~/shared/types/post'
import { descendantIds, refKey } from '~/utils/folder-tree'
import { libraryEntries, type LibraryEntry } from '~/utils/library-entries'

const folderMessages: Record<string, string> = {
  folder_limit: 'フォルダは100個までです。使っていないフォルダを消してから、もう一度お試しください。',
  folder_depth: 'フォルダの入れ子は3段までです。',
  folder_name_taken: '同じ場所に同じ名前のフォルダがあります。別の名前にしてください。',
  folder_cycle: 'フォルダを、自分自身やその中のフォルダには移せません。',
}

// The API names why it refused a folder change; any other failure is a plain retry. Either way the screen is back as it was.
function folderError(error: unknown) {
  const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code ?? ''
  return `${folderMessages[code] ?? '保存できませんでした。もう一度お試しください。'}何も変わっていません。`
}

// Folder counts and filters need every item, so a list is read to its last page.
async function everyPage<T>(read: (query: { cursor?: string }) => Promise<{ items: T[], nextCursor: string }>) {
  const items: T[] = []
  let cursor = ''
  do {
    const page = await read(cursor ? { cursor } : {})
    items.push(...page.items)
    cursor = page.nextCursor
  } while (cursor)
  return items
}

// The folder view's rows, folders and placements. placed maps a refKey to its folder; what it leaves out is unsorted.
// loadError is about reading the lists and message about the last write, so a refused write never makes a loaded list
// look unloaded. Updating, deleting and placing change the screen first and put it back when the API refuses; a new
// folder appears once the API has made it, with its id. A library is loading until its first load ends, so the first
// frame shows the spinner.
export function useLibrary() {
  const entries = ref<LibraryEntry[]>([])
  const folders = ref<Folder[]>([])
  const placed = ref(new Map<string, string>())
  const loading = ref(true)
  const loadError = ref('')
  const message = ref('')
  let generation = 0

  async function load() {
    const current = ++generation
    loading.value = true
    loadError.value = ''
    try {
      const [own, testing, posts, tree] = await Promise.all([
        everyPage(async query => accountListSchema.parse(await $fetch('/api/my/problems', { query }))),
        everyPage(async query => accountListSchema.parse(await $fetch('/api/my/problems', { query: { role: 'tester', ...query } }))),
        everyPage(async query => postListSchema.parse(await $fetch('/api/my/posts', { query }))),
        $fetch('/api/my/folders').then(data => folderTreeSchema.parse(data)),
      ])
      if (current !== generation) return
      // Testers cannot publish or apply to 定期便, so their rows show no destinations, as in the old tab.
      for (const item of testing) delete item.readiness
      entries.value = libraryEntries(own, testing, posts)
      folders.value = tree.folders
      placed.value = new Map(tree.items.map(item => [refKey(item), item.folderId]))
    } catch {
      if (current === generation) loadError.value = '問題と記事を読み込めませんでした。もう一度お試しください。'
    } finally {
      if (current === generation) loading.value = false
    }
  }

  // The folder is added once the API has made it, so it can be opened or filled right away.
  async function createFolder(name: string, parentId: string | null): Promise<Folder | undefined> {
    message.value = ''
    try {
      const folder = folderSchema.parse(await $fetch('/api/my/folders', { method: 'POST', body: { name, parentId } }))
      folders.value = [...folders.value, folder]
      return folder
    } catch (error) {
      message.value = folderError(error)
      return undefined
    }
  }

  async function updateFolder(id: string, name: string, parentId: string | null): Promise<boolean> {
    message.value = ''
    const before = folders.value.find(folder => folder.id === id)
    if (!before) return false
    folders.value = folders.value.map(folder => folder.id === id ? { ...folder, name: name.trim(), parentId } : folder)
    try {
      const folder = folderSchema.parse(await $fetch(`/api/my/folders/${id}`, { method: 'PUT', body: { name, parentId } }))
      folders.value = folders.value.map(item => item.id === id ? folder : item)
      return true
    } catch (error) {
      folders.value = folders.value.map(item => item.id === id ? before : item)
      message.value = folderError(error)
      return false
    }
  }

  // The folders below go too, and what they held becomes unsorted.
  async function deleteFolder(id: string): Promise<boolean> {
    message.value = ''
    const gone = new Set([id, ...descendantIds(folders.value, id)])
    const removed = folders.value.filter(folder => gone.has(folder.id))
    const released = [...placed.value].filter(([, folderId]) => gone.has(folderId))
    folders.value = folders.value.filter(folder => !gone.has(folder.id))
    placed.value = new Map([...placed.value].filter(([, folderId]) => !gone.has(folderId)))
    try {
      await $fetch(`/api/my/folders/${id}`, { method: 'DELETE' })
      return true
    } catch (error) {
      folders.value = [...folders.value, ...removed]
      const back = new Map(placed.value)
      // An item placed again meanwhile keeps its new folder.
      for (const [key, folderId] of released) if (!back.has(key)) back.set(key, folderId)
      placed.value = back
      message.value = folderError(error)
      return false
    }
  }

  // A null folderId makes the items unsorted. The API takes maxFolderItems at a time. When a batch after the first
  // fails, the batches before it are saved, so they stay moved on the screen too and the message says how many moved.
  async function place(refs: FolderRef[], folderId: string | null): Promise<boolean> {
    message.value = ''
    const keys = refs.map(refKey)
    const before = new Map(keys.map(key => [key, placed.value.get(key)]))
    const moved = new Map(placed.value)
    for (const key of keys) {
      if (folderId === null) moved.delete(key)
      else moved.set(key, folderId)
    }
    placed.value = moved
    for (let start = 0; start < refs.length; start += maxFolderItems) {
      const items = refs.slice(start, start + maxFolderItems).map(({ kind, id }) => ({ kind, id }))
      try {
        await $fetch('/api/my/folder-items', { method: 'PUT', body: { folderId, items } })
      } catch (error) {
        const saved = new Set(keys.slice(0, start))
        const back = new Map(placed.value)
        for (const key of keys.slice(start)) {
          if (saved.has(key)) continue
          const previous = before.get(key)
          if (previous === undefined) back.delete(key)
          else back.set(key, previous)
        }
        placed.value = back
        message.value = start === 0 ? folderError(error) : `${refs.length}件中${start}件を移動しました。残りは移動できませんでした。もう一度お試しください。`
        return false
      }
    }
    return true
  }

  return { entries, folders, placed, loading, loadError, message, load, createFolder, updateFolder, deleteFolder, place }
}
