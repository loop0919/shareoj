import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../../utils/private-request'
import { uuidPattern } from '../../../../utils/route-param'
// Cancels the registration before the start. It answers with the contest, as joining does, so it is not a contestAction.
export default defineEventHandler((event) => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  return privateAPI(event, `/my/contests/${id}/participation`, { method: 'DELETE' })
})
