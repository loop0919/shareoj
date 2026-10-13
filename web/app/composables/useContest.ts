import type { Contest } from '~~/shared/types/contest'

// The contest pages and the site header share one request per contest through the key.
export function useContest(id: MaybeRefOrGetter<string>) {
  return useFetch<Contest>(() => `/api/contests/${encodeURIComponent(toValue(id))}`, { key: () => `contest:${toValue(id)}`, dedupe: 'defer' })
}
