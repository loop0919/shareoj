import type { ComputedRef } from 'vue'

// A list whose conditions live in the route changes them by moving to another URL. The router takes a few ticks, and a
// change made in between would build on the route's conditions and undo the one before. So the conditions the latest
// change is moving to are kept here (pending), like usePageQuery's pending page, until the route has them, and each
// change builds on them. The list and its panel still follow the route, so new conditions come with the first page in
// one move.
// conditions are the route's; equal tells whether two of them list the same. navigate makes the move and resolves to
// whether it happened: a move that did not happen hands the conditions back to the route.
export function useConditionChanges<T extends object>(options: {
  conditions: ComputedRef<T>
  equal: (a: T, b: T) => boolean
  navigate: (next: T) => Promise<boolean>
}) {
  const { conditions, equal, navigate } = options
  const pending = shallowRef<T>()
  // The route has moved on, to the conditions on their way or, by the back button, to others: either way it leads again.
  watch(conditions, (next, previous) => { if (!equal(next, previous)) pending.value = undefined }, { flush: 'sync' })
  async function change(patch: Partial<T>) {
    const next = { ...(pending.value ?? conditions.value), ...patch }
    pending.value = next
    let moved = false
    try { moved = await navigate(next) }
    finally { if (!moved && pending.value === next) pending.value = undefined }
  }
  return { change, pending }
}
