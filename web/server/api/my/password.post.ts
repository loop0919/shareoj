import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../utils/private-request'
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  await privateAPI(event, '/my/password', { method: 'POST', body: await limitedJSON(event, 2048) })
  return sendNoContent(event)
})
