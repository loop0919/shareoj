import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  await privateAPI(event, `/my/problems/${id}/featured`, { method: 'PUT', body: await limitedJSON(event, 1024) })
  return sendNoContent(event)
})
