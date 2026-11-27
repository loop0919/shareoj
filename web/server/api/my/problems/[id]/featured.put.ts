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
  await privateAPI(event, `/my/problems/${id}/featured`, { method: 'PUT', body: await limitedJSON(event, 1024) })
  return sendNoContent(event)
})
