import { labelIndex, problemLabel } from '~~/shared/types/contest'

// The contest problem page's submission tabs moved to the contest's lists, narrowed to the problem (ADR 0034). Older
// links to them open those lists with the rest of their query, such as the onsite filter, also when followed from the
// problem page itself, which stays mounted for the query. The letters may still change before the start, so the
// redirect is not permanent. It keeps the navigation's own push or replace, so the back button still returns to the
// page the link was on.
export default defineNuxtRouteMiddleware(to => {
  if (to.query.view !== 'my-submissions' && to.query.view !== 'submissions') return
  const index = labelIndex(to.params.label)
  return navigateTo({ path: `/contests/${String(to.params.id)}`, query: { ...to.query, problem: index < 0 ? undefined : problemLabel(index) }, hash: to.hash }, { redirectCode: 302 })
})
