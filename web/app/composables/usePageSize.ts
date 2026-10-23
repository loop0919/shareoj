export const pageSizes = [20, 50, 100] as const
export type PageSize = typeof pageSizes[number]
type PagedList = 'problems' | 'standings' | 'submissions'
// The standings start small so the FA and solver counts under the table are a short scroll away (ADR 0028).
const initialSizes: Record<PagedList, PageSize> = { problems: 50, standings: 20, submissions: 50 }

// Rows per page of each list, kept per browser in a cookie like the other display settings, so the server renders the
// chosen size too and signed-out viewers keep it. A value other than the choices reads as the list's initial size.
export function usePageSize(list: PagedList) {
  // Without a default, a viewer who never chose gets no cookie.
  const saved = useCookie<unknown>(`openoj-page-size-${list}`, { maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' })
  return computed<PageSize>({
    get: () => pageSizes.find(size => size === saved.value) ?? initialSizes[list],
    set: size => { saved.value = size },
  })
}
