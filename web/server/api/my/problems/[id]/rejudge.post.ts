import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../../utils/private-request'
import { uuidPattern } from '../../../../utils/route-param'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  return privateAPI<{ queued: number }>(event, `/my/problems/${id}/rejudge`, { method: 'POST', body: await limitedJSON(event, 16 * 1024) })
})
