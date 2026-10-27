import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
import type { Submission } from '../../../../shared/types/submission'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  if (event.method !== 'POST') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  const body = await limitedJSON(event, 400 << 10)
  const result = await privateAPI<Submission>(event, '/my/submissions', { method: 'POST', body })
  setResponseStatus(event, 202)
  return result
})
