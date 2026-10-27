import type { Contest } from '~~/shared/types/contest'

// The contest pages and the site header share one request per contest through the key. The header keeps the data
// while the viewer moves between the contest's pages, and Nuxt does not read a key again while it holds data, so a page
// that passes `reread` reads the contest once more when it opens after another page. Otherwise it would show the
// contest as that page found it, such as a problem not yet solved there.
export function useContest(id: MaybeRefOrGetter<string>, reread = false) {
  const contest = useFetch<Contest>(() => `/api/contests/${encodeURIComponent(toValue(id))}`, { key: () => `contest:${toValue(id)}`, dedupe: 'defer' })
  if (!reread || import.meta.server || useNuxtApp().isHydrating || contest.status.value !== 'success') return contest
  return Object.assign(contest.refresh().then(() => contest), contest)
}
