import { defaultSubmissionConditions, listedLanguage, parseSubmissionConditions, submissionConditionQuery, type RuntimeLanguage, type SubmissionConditions } from '~/utils/submission-conditions'

const same = (a: SubmissionConditions, b: SubmissionConditions) => (Object.keys(a) as (keyof SubmissionConditions)[]).every(key => a[key] === b[key])

// The route's conditions as the submission lists, and the links that carry them, read them, with the judge's catalogue
// (languages; [] until it is read). A language or a runtime the catalogue does not list reads as すべて
// (listedLanguage), as does any until the catalogue is first read. The layout reads the catalogue while the server
// renders the page, so a list starts with it; only when that read failed does a list ask before it, without the
// language, and once more when the catalogue comes and lists it. A read that fails later keeps the languages last read.
export function useRouteSubmissionConditions() {
  const route = useRoute()
  const { data: catalog } = useJudgeCatalog()
  const known = computed<RuntimeLanguage[] | undefined>(previous => catalog.value?.languages ?? previous)
  const conditions = computed(() => listedLanguage(parseSubmissionConditions(route.query), known.value))
  const languages = computed(() => known.value ?? [])
  return { conditions, languages }
}

// The sort and filters of a submission list, kept in the URL so a reload or a shared link opens the same list. Nothing
// is remembered for the next visit.
// keep names the page's queries that are not conditions, such as view; they stay through every change. A change of
// conditions replaces the history entry, as the list's other controls do, and drops the page number: the list starts
// over. Without withUser, as in a viewer's own submissions, the list has no submitter: one in the URL is neither shown
// nor sent, and a change leaves it out, as it leaves out a language the catalogue does not list.
export function useSubmissionConditions(options: { keep: string[], withUser: () => boolean }) {
  const route = useRoute()
  const router = useRouter()
  const { conditions: read, languages } = useRouteSubmissionConditions()
  // The route also changes with what the conditions leave out, and the catalogue is read again now and then. The
  // conditions keep their object while they stay the same, so what is built on them, such as the list's query, changes
  // only with them.
  const conditions = computed<SubmissionConditions>(previous => {
    const next = options.withUser() ? read.value : { ...read.value, user: '' }
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
  return { conditions, change, clear, query, languages }
}
