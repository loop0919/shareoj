import { folderSchema, type FolderRef } from '~~/shared/types/folders'

// How long a first save waits for its placement before it gives up on it.
export const placementTimeoutMs = 5000

// A problem or post made from inside a folder on the my page goes into it once its first save succeeds. The editors
// pass their ?folder= as it is and read no folders: anything but a UUID names none, the API ignores a folder that is
// not the caller's, and a failure or a placement slower than placementTimeoutMs is dropped, since the save went through
// and the item shows in 未整理.
export async function placeNewItem(folder: unknown, item: FolderRef): Promise<void> {
  const folderId = folderSchema.shape.id.safeParse(folder)
  if (!folderId.success) return
  await $fetch('/api/my/folder-items', { method: 'PUT', body: { folderId: folderId.data, items: [item] }, timeout: placementTimeoutMs }).catch(() => {})
}
