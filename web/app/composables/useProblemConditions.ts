import { conditionQuery, defaultConditions, parseConditions, type ProblemConditions } from '~/utils/problem-conditions'

// The sort and filters of the problem list, kept in the URL so a reload, the back button or a shared link opens the same
// list. A change opens the list as a new history entry, on its first page: the page number belongs to the old list.
// Call it from the list page, so the conditions and usePageQuery's page come from the same route and change together.
export function useProblemConditions() {
  const route = useRoute()
  // The route also changes with the page alone. The conditions keep their object while they stay the same, so what is
  // built on them, such as the list's query, changes only with them and a new page loads once.
  const conditions = computed<ProblemConditions>(previous => {
    const next = parseConditions(route.query)
    return previous && (Object.keys(next) as (keyof ProblemConditions)[]).every(key => next[key] === previous[key]) ? previous : next
  })
  async function open(next: ProblemConditions) {
    await navigateTo({ path: '/problems', query: conditionQuery(next) })
  }
  // A bound moved past the other one carries it along, so the range stays one.
  function change(patch: Partial<ProblemConditions>) {
    const next = { ...conditions.value, ...patch }
    if (next.min > next.max) {
      if (patch.min === undefined) next.min = next.max
      else next.max = next.min
    }
    return open(next)
  }
  const clear = () => open(defaultConditions)
  return { conditions, change, clear }
}
