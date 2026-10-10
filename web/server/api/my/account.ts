import { privateAPI } from '../../utils/private-api'
import { clearPrivateSession, hasSession } from '../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../utils/private-request'
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  if (event.method === 'GET') return privateAPI(event, '/my/account')
  if (event.method !== 'DELETE') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  await privateAPI(event, '/my/account', { method: 'DELETE', body: await limitedJSON(event, 1024) })
  // The sign-in no longer exists, so its cookies must not outlive it.
  clearPrivateSession(event)
  return sendNoContent(event)
})
