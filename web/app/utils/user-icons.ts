import { reactive } from 'vue'
import { iconBatchSize } from '~~/shared/types/profile'

export interface UserIcon { handle: string, avatar: string }

// Collects the handles a page shows and reads their icons together. A list of people used to cost one request per
// name, all at once, which is more than the API runs at a time. `fetchIcons` answers the handles that have a profile;
// `schedule` runs the send once the names rendered together have all asked.
export function createIconLoader(
  fetchIcons: (handles: string[]) => Promise<UserIcon[]>,
  schedule: (send: () => void) => void = send => { setTimeout(send, 0) },
) {
  // The icon of each handle that has been answered; '' means it has none.
  const icons = reactive(new Map<string, string>())
  const waiting = new Set<string>()
  const flying = new Set<string>()
  function send() {
    const handles = [...waiting]
    waiting.clear()
    for (let start = 0; start < handles.length; start += iconBatchSize) {
      const batch = handles.slice(start, start + iconBatchSize)
      for (const handle of batch) flying.add(handle)
      fetchIcons(batch).then(items => {
        const found = new Map(items.map(item => [item.handle, item.avatar]))
        for (const handle of batch) icons.set(handle, found.get(handle) ?? '')
      }).catch(() => {
        // The handles stay unknown, so the next page that shows them asks again.
      }).finally(() => { for (const handle of batch) flying.delete(handle) })
    }
  }
  function want(handle: string) {
    if (icons.has(handle) || flying.has(handle) || waiting.has(handle)) return
    if (!waiting.size) schedule(send)
    waiting.add(handle)
  }
  return { icons, want }
}
