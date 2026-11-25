import { folderSchema, maxFolderDepth, type Folder, type FolderRef } from '~~/shared/types/folders'

// The key a problem or post goes by in placements and selections; a problem's id never names a post.
export const refKey = (ref: FolderRef) => `${ref.kind}:${ref.id}`

export function sortedChildren(folders: Folder[], parentId: string | null): Folder[] {
  return folders.filter(folder => folder.parentId === parentId)
    .sort((a, b) => a.name.localeCompare(b.name, 'ja') || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
}

// From the top down to the folder; an unknown id has no path. The seen set stops a loop the API never sends.
export function folderPath(folders: Folder[], id: string): Folder[] {
  const byId = new Map(folders.map(folder => [folder.id, folder]))
  const path: Folder[] = []
  const seen = new Set<string>()
  for (let folder = byId.get(id); folder && !seen.has(folder.id); folder = folder.parentId === null ? undefined : byId.get(folder.parentId)) {
    seen.add(folder.id)
    path.unshift(folder)
  }
  return path
}

// Every folder below, at any level, without the folder itself.
export function descendantIds(folders: Folder[], id: string): string[] {
  const found = new Set<string>()
  const queue = [id]
  for (let parent = queue.shift(); parent !== undefined; parent = queue.shift()) {
    for (const folder of folders) {
      if (folder.parentId !== parent || folder.id === id || found.has(folder.id)) continue
      found.add(folder.id)
      queue.push(folder.id)
    }
  }
  return [...found]
}

// Whether a folder, with everything below it, fits under the parent; null for id is a new folder and null for
// parentId the top. Like the API, a folder cannot go into itself or below itself, nor reach past maxFolderDepth.
export function canNest(folders: Folder[], id: string | null, parentId: string | null): boolean {
  const below = id === null ? [] : descendantIds(folders, id)
  if (id !== null && (id === parentId || (parentId !== null && below.includes(parentId)))) return false
  const parentDepth = parentId === null ? 0 : folderPath(folders, parentId).length
  if (parentId !== null && parentDepth === 0) return false
  // The levels the folder takes: its own, and the deepest of those below it, counted from it.
  const depth = id === null ? 0 : folderPath(folders, id).length
  const levels = 1 + Math.max(0, ...below.map(lower => folderPath(folders, lower).length - depth))
  return parentDepth + levels <= maxFolderDepth
}

// placed maps a refKey to its folder and keys lists everything shown. A folder counts what its lower folders hold too,
// and a placement in a folder that is not listed counts as unsorted, since no folder shows it.
export function folderCounts(folders: Folder[], placed: Map<string, string>, keys: string[]): { all: number, unsorted: number, byFolder: Map<string, number> } {
  const byFolder = new Map(folders.map(folder => [folder.id, 0]))
  const paths = new Map<string, Folder[]>()
  let unsorted = 0
  for (const key of keys) {
    const folderId = placed.get(key)
    if (folderId === undefined || !byFolder.has(folderId)) { unsorted++; continue }
    if (!paths.has(folderId)) paths.set(folderId, folderPath(folders, folderId))
    for (const folder of paths.get(folderId)!) byFolder.set(folder.id, byFolder.get(folder.id)! + 1)
  }
  return { all: keys.length, unsorted, byFolder }
}

// A problem or post made from inside a folder on the my page goes into it once its first save succeeds. The editors
// pass their ?folder= as it is and read no folders: anything but a UUID names none, the API ignores a folder that is
// not the caller's, and a failure is dropped, since the save went through and the item shows in 未整理.
export async function placeNewItem(folder: unknown, item: FolderRef): Promise<void> {
  const folderId = folderSchema.shape.id.safeParse(folder)
  if (!folderId.success) return
  await $fetch('/api/my/folder-items', { method: 'PUT', body: { folderId: folderId.data, items: [item] } }).catch(() => {})
}
