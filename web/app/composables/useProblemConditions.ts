import { conditionQuery, defaultConditions, parseConditions, rememberedQuery, type ProblemConditions } from '~/utils/problem-conditions'

// The conditions the list remembers; the tag and the page number are not among them.
const rememberedKeys = ['sort', 'solved', 'contest', 'min', 'max'] as const

// The sort and filters of the problem list, kept in the URL so a reload, the back button or a shared link opens the same
// list. A change opens the list as a new history entry, on its first page: the page number belongs to the old list.
// The conditions the viewer last chose are also kept per browser in a cookie, like the page size, and a /problems without
// any query opens with them, the server's first render included. A URL with a query, a page number alone too, opens as
// it is.
// Call it from the list page, so the conditions and usePageQuery's page come from the same route and change together.
export function useProblemConditions() {
  const route = useRoute()
  // Without a default, a viewer who never chose gets no cookie. It holds rememberedQuery's shape, but it is read like a
  // URL, so whatever else it holds reads as the initial conditions. Nuxt's own decoding throws on a broken percent
  // escape and fails the page; this one hands such a value on as it is.
  const saved = useCookie<unknown>('openoj-problem-conditions', {
    maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/',
    decode: value => { try { return value && JSON.parse(decodeURIComponent(value)) } catch { return value } },
  })
  const remembered = () => rememberedQuery(parseConditions(saved.value))
  // A bare /problems is on its way to the remembered conditions (restore), so it lists them already: the list loads once.
  // The route also changes with the page alone. The conditions keep their object while they stay the same, so what is
  // built on them, such as the list's query, changes only with them and a new page loads once.
  const conditions = computed<ProblemConditions>(previous => {
    const next = parseConditions(Object.keys(route.query).length ? route.query : remembered())
    return previous && (Object.keys(next) as (keyof ProblemConditions)[]).every(key => next[key] === previous[key]) ? previous : next
  })
  async function open(next: ProblemConditions) {
    await navigateTo({ path: '/problems', query: conditionQuery(next) })
  }
  // Nothing left to remember removes the cookie. A signed-out viewer's solved is kept, to apply once they sign in.
  function remember(next: ProblemConditions) {
    const query = rememberedQuery(next)
    saved.value = Object.keys(query).length ? query : null
  }
  // A bound moved past the other one carries it along, so the range stays one. Only a change to a remembered condition
  // rewrites what is remembered; a change of tag alone leaves it.
  function change(patch: Partial<ProblemConditions>) {
    const next = { ...conditions.value, ...patch }
    if (next.min > next.max) {
      if (patch.min === undefined) next.min = next.max
      else next.max = next.min
    }
    if (rememberedKeys.some(key => next[key] !== conditions.value[key])) remember(next)
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
