import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  const body = await limitedJSON(event, 2 << 10)
  return privateAPI(event, `/my/problems/${id}/test-files`, { method: 'POST', body })
})
