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
  if (!streamingMode(event) || !submission.result) return submission
  // The API adds the checker diagnostics to a problem manager's own submission; they can reveal tests and checker
  // source. Only the sample check of an interactive or two-step problem carries diagnostics the API shows to everyone,
  // so only those stay. A generate job's cases carry the author's test outputs, which also go.
  const { checkerLog: _checkerLog, ...withoutLog } = submission.result
  const result = submission.easyTest && submission.result.interactive ? submission.result : withoutLog
  const cases = result.cases?.map(({ output: _output, outputFile: _outputFile, ...kept }) => kept)
  return { ...submission, result: cases ? { ...result, cases } : result }
})
