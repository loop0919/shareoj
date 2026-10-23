import type { RouteLocationRaw } from 'vue-router'

// The page of a paged list, kept in ?page= so a reload, the back button or a shared link opens the same page (ADR 0028).
// The first page leaves it out. Anything but a whole number from 1 asks for the first page; the list clamps the rest.
export function usePageQuery() {
  const route = useRoute()
  const router = useRouter()
  const requested = computed(() => {
    const page = route.query.page
    return typeof page === 'string' && /^\d+$/.test(page) ? Math.max(Number(page), 1) : 1
  })
  const link = (page: number): RouteLocationRaw => ({ path: route.path, query: { ...route.query, page: page > 1 ? String(page) : undefined } })
  async function go(page: number) { await router.replace(link(page)) }
  return { requested, link, go }
}
