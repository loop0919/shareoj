import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { privateHeaders } from '../../../utils/private-request'
import { streamingMode } from '../../../utils/streaming-mode'
import type { Submission } from '../../../../shared/types/submission'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(id)) throw createError({ statusCode: 404 })
  const submission = await privateAPI<Submission>(event, `/my/submissions/${id}`, { streaming: true })
  // The API adds the checker diagnostics to an author's or tester's own submission; they can reveal tests and checker
  // source. A sample check's diagnostics are public, so they stay.
  if (streamingMode(event) && !submission.easyTest && submission.result) {
    const { checkerLog: _checkerLog, ...result } = submission.result
    return { ...submission, result }
  }
  return submission
})
