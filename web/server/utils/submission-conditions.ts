import type { H3Event } from 'h3'
import { isLanguageKey, isRuntimeId, isSubmissionUser, submissionSortKeys, submissionVerdicts } from '~~/shared/utils/submission-conditions'

// Adds the sort and filter queries of a submission list to the API's query, each only when its value is valid.
// A value that cannot be valid is dropped, like a malformed problem filter. The runtime wins over the language,
// which the API would refuse together.
export function setSubmissionConditions(event: H3Event, search: URLSearchParams) {
  const query = getQuery(event)
  const value = (key: string, valid: (value: string) => boolean) => {
    const given = query[key]
    return typeof given === 'string' && valid(given) ? given : ''
  }
  const runtime = value('runtime', isRuntimeId)
  const conditions = {
    sort: value('sort', v => submissionSortKeys.some(key => key === v)),
    verdict: value('verdict', v => submissionVerdicts.includes(v)),
    language: runtime ? '' : value('language', isLanguageKey),
    runtime,
    user: value('user', isSubmissionUser),
  }
  for (const [key, v] of Object.entries(conditions)) if (v) search.set(key, v)
}
