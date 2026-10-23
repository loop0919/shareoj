import type { Ref } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { pageAfterResize } from '~/utils/pagination'
import type { PageSize } from '~/composables/usePageSize'

// The page of a paged list, kept in ?page= so a reload, the back button or a shared link opens the same page (ADR 0028).
// The first page leaves it out. Anything but a whole number from 1 asks for the first page; the list clamps the rest.
// Call it once per page, so go() and resize() share the page they are moving to.
export function usePageQuery() {
  const route = useRoute()
  const router = useRouter()
  const inRoute = computed(() => {
    const page = route.query.page
    return typeof page === 'string' && /^\d+$/.test(page) ? Math.max(Number(page), 1) : 1
  })
  // The page go() is moving to, until the route has it. The router takes a few ticks, and the page's own route a render
  // more, so without it a new size would meet the old page for a render and a list would fetch that page in between.
  const pending = ref<number>()
  watch(inRoute, () => { pending.value = undefined }, { flush: 'sync' })
  const requested = computed(() => pending.value ?? inRoute.value)
  const link = (page: number): RouteLocationRaw => ({ path: route.path, query: { ...route.query, page: page > 1 ? String(page) : undefined } })
  async function go(page: number) {
    pending.value = page
    let moved = false
    // A navigation that did not happen hands the page back to the route.
    try { moved = !await router.replace(link(page)) }
    finally { if (!moved && pending.value === page) pending.value = undefined }
  }
  // Moves the list from page, shown at size rows a page, to the page holding the same first row at to rows a page. The
  // size and the page change in the same tick, so a list keyed on both sees one change.
  function resize(size: Ref<PageSize>, page: number, to: PageSize) {
    const target = pageAfterResize(page, size.value, to)
    size.value = to
    return go(target)
  }
  return { requested, link, go, resize }
}
