import { defaultSubmissionConditions, parseSubmissionConditions, submissionConditionQuery, type SubmissionConditions } from '~/utils/submission-conditions'

const same = (a: SubmissionConditions, b: SubmissionConditions) => (Object.keys(a) as (keyof SubmissionConditions)[]).every(key => a[key] === b[key])

// The sort and filters of a submission list, kept in the URL so a reload or a shared link opens the same list. Nothing
// is remembered for the next visit.
// keep names the page's queries that are not conditions, such as view; they stay through every change. A change of
// conditions replaces the history entry, as the list's other controls do, and drops the page number: the list starts
// over. Without withUser, as in a viewer's own submissions, the list has no submitter: one in the URL is neither shown
// nor sent, and a change leaves it out.
export function useSubmissionConditions(options: { keep: string[], withUser: () => boolean }) {
  const route = useRoute()
  const router = useRouter()
  // The route also changes with what the conditions leave out. The conditions keep their object while they stay the
  // same, so what is built on them, such as the list's query, changes only with them.
  const conditions = computed<SubmissionConditions>(previous => {
    const read = parseSubmissionConditions(route.query)
    const next = options.withUser() ? read : { ...read, user: '' }
    return previous && same(next, previous) ? previous : next
  })
  // The router itself, not navigateTo: while another move runs Nuxt's route middleware, navigateTo takes itself for a
  // middleware's answer and does not move.
  const { change } = useConditionChanges({
    conditions,
    equal: same,
    navigate: async next => {
      const kept = Object.fromEntries(options.keep.filter(key => route.query[key] !== undefined).map(key => [key, route.query[key]]))
      return !await router.replace({ query: { ...kept, ...submissionConditionQuery(next) } })
    },
  })
  const clear = () => change(defaultSubmissionConditions)
  const query = computed(() => submissionConditionQuery(conditions.value))
  return { conditions, change, clear, query }
}
