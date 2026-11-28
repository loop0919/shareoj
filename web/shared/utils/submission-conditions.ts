import { userHandle } from '../types/profile'

// The values of the submission lists' sort and filter queries. The pages and the server routes read them alike.
export type SubmissionSort = 'newest' | 'oldest' | 'shortest' | 'fastest'
export const submissionSortKeys: readonly SubmissionSort[] = ['newest', 'oldest', 'shortest', 'fastest']
export const submissionVerdicts: readonly string[] = ['AC', 'WA', 'TLE', 'MLE', 'OLE', 'RE', 'CE', 'JE']

// A language key and a runtime ID are told apart only by the parameter they arrive in; the API checks the content.
// A runtime ID never carries the isolate or local suffix.
const keyShape = /^[a-z][a-z0-9-]*$/
export const isLanguageKey = (value: string) => keyShape.test(value)
export const isRuntimeId = (value: string) => keyShape.test(value) && !value.endsWith('-isolate') && !value.endsWith('-local')
// The submitter's user ID is the handle.
export const isSubmissionUser = (value: string) => userHandle.test(value)
