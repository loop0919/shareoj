import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  return privateAPI<{ queued: number }>(event, `/my/problems/${id}/rejudge`, { method: 'POST', body: await limitedJSON(event, 16 * 1024) })
})
