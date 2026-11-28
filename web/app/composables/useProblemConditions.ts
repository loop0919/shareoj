import { conditionQuery, defaultConditions, parseConditions, rememberedQuery, type ProblemConditions } from '~/utils/problem-conditions'

// The conditions the list remembers; the tag and the page number are not among them.
const rememberedKeys = ['sort', 'solved', 'contest', 'min', 'max'] as const
const same = (a: ProblemConditions, b: ProblemConditions) => (Object.keys(a) as (keyof ProblemConditions)[]).every(key => a[key] === b[key])

// The sort and filters of the problem list, kept in the URL so a reload, the back button or a shared link opens the same
// list. A change opens the list as a new history entry, on its first page: the page number belongs to the old list.
// The conditions the viewer last chose are also kept per browser in a cookie, like the page size, and a /problems without
// any query opens with them, the server's first render included. A URL with a query, a page number alone too, opens as
// it is.
// Call it from the list page, so the conditions and usePageQuery's page come from the same route and change together.
export function useProblemConditions() {
  const route = useRoute()
  const router = useRouter()
  // Without a default, a viewer who never chose gets no cookie. It holds rememberedQuery's shape, but it is read like a
  // URL, so whatever else it holds reads as the initial conditions. Nuxt's own decoding throws on a broken percent
  // escape and fails the page; this one hands such a value on as it is.
  const saved = useCookie<unknown>('openoj-problem-conditions', {
    maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/',
    decode: value => { try { return value && JSON.parse(decodeURIComponent(value)) } catch { return value } },
  })
  const remembered = () => rememberedQuery(parseConditions(saved.value))
  // The conditions a list with this query shows. A bare /problems is on its way to the remembered conditions (restore),
  // so it lists them already: the list loads once.
  const listed = (query: Record<string, unknown>) => parseConditions(Object.keys(query).length ? query : remembered())
  // The route also changes with the page alone. The conditions keep their object while they stay the same, so what is
  // built on them, such as the list's query, changes only with them and a new page loads once.
  const conditions = computed<ProblemConditions>(previous => {
    const next = listed(route.query)
    return previous && same(next, previous) ? previous : next
  })
  // The conditions the latest change is moving to, until the route has them, like usePageQuery's pending page. The
  // router takes a few ticks, and a change made in between builds on these: on the route's, it would undo the one before.
  // The list and the panel still follow the route, so new conditions come with the first page in one move.
  const pending = shallowRef<ProblemConditions>()
  watch(conditions, () => { pending.value = undefined }, { flush: 'sync' })
  // The router itself, not navigateTo: while another move runs Nuxt's route middleware, navigateTo takes itself for a
  // middleware's answer and does not move. A move that did not happen hands the conditions back to the route.
  async function open(next: ProblemConditions) {
    const query = conditionQuery(next)
    const target = listed(query)
    pending.value = target
    let moved = false
    try { moved = !await router.push({ path: '/problems', query }) }
    finally { if (!moved && pending.value === target) pending.value = undefined }
  }
  // Nothing left to remember removes the cookie. A signed-out viewer's solved is kept, to apply once they sign in.
  function remember(next: ProblemConditions) {
    const query = rememberedQuery(next)
    saved.value = Object.keys(query).length ? query : null
  }
  // A bound moved past the other one carries it along, so the range stays one. Only a change to a remembered condition
  // rewrites what is remembered; a change of tag alone leaves it.
  function change(patch: Partial<ProblemConditions>) {
    const base = pending.value ?? conditions.value
    const next = { ...base, ...patch }
    if (next.min > next.max) {
      if (patch.min === undefined) next.min = next.max
      else next.max = next.min
    }
    if (rememberedKeys.some(key => next[key] !== base[key])) remember(next)
    return open(next)
  }
  function clear() {
    saved.value = null
    return open(defaultConditions)
  }
  // On a /problems without any query, replaces the URL with the remembered conditions. Resolves to nothing when there is
  // none. The server answers with a redirect; the browser replaces the history entry, so the back button skips it.
  function restore() {
    if (Object.keys(route.query).length) return
    const query = remembered()
    if (Object.keys(query).length) return navigateTo({ path: '/problems', query }, { replace: true, redirectCode: 302 })
  }
  return { conditions, change, clear, restore }
}
