import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../../utils/private-request'
import { uuidPattern } from '../../../../utils/route-param'
// POST shows the contest's co-setter link, creating it on first use; PUT replaces it. Neither takes a body.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  if (event.method !== 'POST' && event.method !== 'PUT') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  return privateAPI(event, `/my/contests/${id}/setter-invitation`, { method: event.method })
})
