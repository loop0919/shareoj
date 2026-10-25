import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
import type { CodeRun } from '../../../../shared/types/code-run'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  // Source up to 128 KiB plus standard input up to 1 MiB, which JSON escaping can inflate.
  const body = await limitedJSON(event, 3 << 20)
  const result = await privateAPI<CodeRun>(event, '/my/code-runs', { method: 'POST', body })
  setResponseStatus(event, 202)
  return result
})
